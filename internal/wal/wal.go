package wal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sync"
)

var ErrTruncateIncomplete = errors.New("wal: truncate copy incomplete")

// Record represents a single entry in the WAL.
type Record struct {
	Key       []byte
	Value     []byte
	Tombstone bool
}

// WAL manages the write-ahead log file.
type WAL struct {
	path   string
	file   *os.File
	mu     sync.Mutex
	limits ReplayLimits
}

// Open creates or opens the WAL file at the given path.
func Open(path string) (*WAL, error) {
	return OpenWithLimits(path, DefaultReplayLimits())
}

// OpenWithLimits opens a WAL with append size validation.
func OpenWithLimits(path string, limits ReplayLimits) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &WAL{path: path, file: f, limits: limits.WithDefaults()}, nil
}

// Append writes a record to the WAL with checksum.
// Format: keyLen(4) | key | valueLen(4) | value | tombstone(1) | checksum(4)
func (w *WAL) Append(rec Record) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err := w.appendRecordLocked(rec)
	return err
}

// AppendBatch writes multiple records sequentially and fsyncs once at the end.
// If any write or Sync fails, the batch is not considered durable; callers must
// not apply records to the memtable.
//
// BeforeBatchSync, when set, runs immediately before fsync (tests only).
var BeforeBatchSync func()

func (w *WAL) AppendBatch(records []Record) error {
	if len(records) == 0 {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, rec := range records {
		if _, err := w.appendRecordLocked(rec); err != nil {
			return err
		}
	}
	if BeforeBatchSync != nil {
		BeforeBatchSync()
	}
	return w.file.Sync()
}

func (w *WAL) appendRecordLocked(rec Record) (int, error) {
	buf, err := encodeRecord(rec, w.limits)
	if err != nil {
		return 0, err
	}
	n, err := w.file.Write(buf)
	return n, err
}

func encodeRecord(rec Record, limits ReplayLimits) ([]byte, error) {
	keyLen := len(rec.Key)
	valueLen := len(rec.Value)
	if err := limits.validateRecord(uint32(keyLen), uint32(valueLen)); err != nil {
		return nil, err
	}

	tombByte := byte(0)
	if rec.Tombstone {
		tombByte = 1
	}

	buf := make([]byte, 0, 4+keyLen+4+valueLen+1+4)
	buf = binary.BigEndian.AppendUint32(buf, uint32(keyLen))
	buf = append(buf, rec.Key...)
	buf = binary.BigEndian.AppendUint32(buf, uint32(valueLen))
	buf = append(buf, rec.Value...)
	buf = append(buf, tombByte)
	checksum := crc32.ChecksumIEEE(buf)
	buf = binary.BigEndian.AppendUint32(buf, checksum)
	return buf, nil
}

// Sync ensures all writes are flushed to disk.
func (w *WAL) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Sync()
}

// Close closes the WAL file.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

// Replay reads all records from the WAL and calls the given function for each.
// It verifies checksums and stops on any corruption.
func Replay(path string, fn func(Record) error) error {
	return ReplayWithLimits(path, DefaultReplayLimits(), fn)
}

// ReplayWithLimits replays a WAL file with size bounds to prevent OOM on corrupt data.
// A trailing partial record (crash mid-write) is truncated and ignored.
func ReplayWithLimits(path string, limits ReplayLimits, fn func(Record) error) error {
	_, err := ReplayFromWithRecovery(path, limits, 0, fn)
	return err
}

// ReplayFromWithRecovery replays WAL records starting at startOffset.
// If the file ends with an incomplete record, the file is truncated to the last
// valid byte and replay succeeds.
func ReplayFromWithRecovery(path string, limits ReplayLimits, startOffset int64, fn func(Record) error) (int64, error) {
	limits = limits.WithDefaults()

	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if fi.Size() > limits.MaxFileSize {
		return 0, ErrWALTooLarge
	}
	if startOffset > fi.Size() {
		startOffset = fi.Size()
	}
	if _, err := f.Seek(startOffset, io.SeekStart); err != nil {
		return 0, err
	}

	validEnd := startOffset
	for {
		recordStart, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			return validEnd, err
		}

		rec, n, err := readOneRecord(f, limits)
		if err == io.EOF {
			break
		}
		if err == io.ErrUnexpectedEOF {
			if truncErr := f.Truncate(validEnd); truncErr != nil {
				return validEnd, truncErr
			}
			break
		}
		if err != nil {
			return validEnd, err
		}
		validEnd = recordStart + n
		if err := fn(rec); err != nil {
			return validEnd, err
		}
	}
	return validEnd, nil
}

func readOneRecord(f *os.File, limits ReplayLimits) (Record, int64, error) {
	start, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return Record{}, 0, err
	}

	var keyLen uint32
	if err := binary.Read(f, binary.BigEndian, &keyLen); err != nil {
		return Record{}, 0, err
	}
	if keyLen > limits.MaxKeySize {
		return Record{}, 0, ErrKeyTooLarge
	}

	key := make([]byte, keyLen)
	if _, err := io.ReadFull(f, key); err != nil {
		return Record{}, 0, mapRecordEOF(err, start)
	}

	var valueLen uint32
	if err := binary.Read(f, binary.BigEndian, &valueLen); err != nil {
		return Record{}, 0, mapRecordEOF(err, start)
	}
	if valueLen > limits.MaxValueSize {
		return Record{}, 0, ErrValueTooLarge
	}
	if err := limits.validateRecord(keyLen, valueLen); err != nil {
		return Record{}, 0, err
	}

	value := make([]byte, valueLen)
	if _, err := io.ReadFull(f, value); err != nil {
		return Record{}, 0, mapRecordEOF(err, start)
	}

	var tombByte byte
	if err := binary.Read(f, binary.BigEndian, &tombByte); err != nil {
		return Record{}, 0, mapRecordEOF(err, start)
	}

	var checksum uint32
	if err := binary.Read(f, binary.BigEndian, &checksum); err != nil {
		return Record{}, 0, mapRecordEOF(err, start)
	}

	end, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return Record{}, 0, err
	}

	buf := make([]byte, 0, 4+int(keyLen)+4+int(valueLen)+1)
	buf = binary.BigEndian.AppendUint32(buf, keyLen)
	buf = append(buf, key...)
	buf = binary.BigEndian.AppendUint32(buf, valueLen)
	buf = append(buf, value...)
	buf = append(buf, tombByte)
	if crc32.ChecksumIEEE(buf) != checksum {
		return Record{}, 0, io.ErrUnexpectedEOF
	}

	return Record{
		Key:       key,
		Value:     value,
		Tombstone: tombByte == 1,
	}, end - start, nil
}

func mapRecordEOF(err error, recordStart int64) error {
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return io.ErrUnexpectedEOF
	}
	_ = recordStart
	return err
}

// Offset returns the current write position in the WAL file.
func (w *WAL) Offset() (uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	off, err := w.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	return uint64(off), nil
}

// Size returns the current WAL file size in bytes.
func (w *WAL) Size() (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	fi, err := w.file.Stat()
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// Truncate clears the entire WAL file.
func (w *WAL) Truncate() error {
	return w.TruncateBefore(1 << 62)
}

// TruncateBefore removes the first truncateAt bytes and keeps the remainder.
// Used after flush so records for the new active memtable are preserved.
//
// Implementation copies the tail into a temp file, fsyncs, then atomically
// replaces wal.log so a crash mid-truncate cannot corrupt the original WAL.
func (w *WAL) TruncateBefore(truncateAt int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if truncateAt <= 0 {
		return nil
	}

	if err := w.file.Sync(); err != nil {
		return err
	}

	fi, err := w.file.Stat()
	if err != nil {
		return err
	}
	size := fi.Size()
	if truncateAt >= size {
		return w.reopenEmptyLocked()
	}

	tmpPath := w.path + ".truncate.tmp"
	if err := w.copyWalTailLocked(truncateAt, size, tmpPath); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if err := w.file.Close(); err != nil {
		os.Remove(tmpPath)
		return w.reopenAppendAfterTruncateErr(err)
	}
	w.file = nil

	if err := os.Rename(tmpPath, w.path); err != nil {
		os.Remove(tmpPath)
		if reopenErr := w.reopenAppend(); reopenErr != nil {
			return errors.Join(err, reopenErr)
		}
		return err
	}

	return w.reopenAppend()
}

func (w *WAL) copyWalTailLocked(truncateAt, size int64, tmpPath string) error {
	tmp, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer tmp.Close()

	const chunkSize = 64 * 1024
	buf := make([]byte, chunkSize)
	for off := truncateAt; off < size; {
		n := int64(len(buf))
		if size-off < n {
			n = size - off
		}
		readN, err := w.file.ReadAt(buf[:n], off)
		if err != nil && err != io.EOF {
			return err
		}
		if readN == 0 {
			return fmt.Errorf("%w: read 0 bytes at offset %d", ErrTruncateIncomplete, off)
		}
		if _, err := tmp.Write(buf[:readN]); err != nil {
			return err
		}
		off += int64(readN)
	}
	return tmp.Sync()
}

func (w *WAL) reopenAppend() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	w.file = f
	_, err = w.file.Seek(0, io.SeekEnd)
	return err
}

func (w *WAL) reopenAppendAfterTruncateErr(cause error) error {
	if err := w.reopenAppend(); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (w *WAL) reopenEmptyLocked() error {
	if err := w.file.Close(); err != nil {
		return err
	}
	if err := os.Truncate(w.path, 0); err != nil {
		return err
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	w.file = f
	return nil
}
