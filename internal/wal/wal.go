package wal

import (
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"sync"
)

// Record represents a single entry in the WAL.
type Record struct {
	Key       []byte
	Value     []byte
	Tombstone bool
}

// WAL manages the write-ahead log file.
type WAL struct {
	file *os.File
	mu   sync.Mutex
}

// Open creates or opens the WAL file at the given path.
func Open(path string) (*WAL, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &WAL{file: f}, nil
}

// Append writes a record to the WAL with checksum.
// Format: keyLen(4) | key | valueLen(4) | value | tombstone(1) | checksum(4)
func (w *WAL) Append(rec Record) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Calculate sizes
	keyLen := len(rec.Key)
	valueLen := len(rec.Value)
	tombByte := byte(0)
	if rec.Tombstone {
		tombByte = 1
	}

	// Buffer for writing (pre-allocate approximate size)
	buf := make([]byte, 0, 4+keyLen+4+valueLen+1+4)

	// Write key length and key
	buf = binary.BigEndian.AppendUint32(buf, uint32(keyLen))
	buf = append(buf, rec.Key...)

	// Write value length and value
	buf = binary.BigEndian.AppendUint32(buf, uint32(valueLen))
	buf = append(buf, rec.Value...)

	// Write tombstone flag
	buf = append(buf, tombByte)

	// Compute checksum over everything except the checksum field
	checksum := crc32.ChecksumIEEE(buf)
	buf = binary.BigEndian.AppendUint32(buf, checksum)

	_, err := w.file.Write(buf)
	if err != nil {
		return err
	}
	return nil
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
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no WAL, nothing to replay
		}
		return err
	}
	defer f.Close()

	for {
		var keyLen uint32
		err := binary.Read(f, binary.BigEndian, &keyLen)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		key := make([]byte, keyLen)
		if _, err := io.ReadFull(f, key); err != nil {
			return err
		}

		var valueLen uint32
		if err := binary.Read(f, binary.BigEndian, &valueLen); err != nil {
			return err
		}
		value := make([]byte, valueLen)
		if _, err := io.ReadFull(f, value); err != nil {
			return err
		}

		var tombByte byte
		if err := binary.Read(f, binary.BigEndian, &tombByte); err != nil {
			return err
		}

		var checksum uint32
		if err := binary.Read(f, binary.BigEndian, &checksum); err != nil {
			return err
		}

		// Recompute checksum to verify
		buf := make([]byte, 0, 4+int(keyLen)+4+int(valueLen)+1)
		buf = binary.BigEndian.AppendUint32(buf, keyLen)
		buf = append(buf, key...)
		buf = binary.BigEndian.AppendUint32(buf, valueLen)
		buf = append(buf, value...)
		buf = append(buf, tombByte)
		if crc32.ChecksumIEEE(buf) != checksum {
			return io.ErrUnexpectedEOF // corruption
		}

		rec := Record{
			Key:       key,
			Value:     value,
			Tombstone: tombByte == 1,
		}
		if err := fn(rec); err != nil {
			return err
		}
	}
	return nil
}

// Truncate clears the WAL file (called after successful flush).
func (w *WAL) Truncate() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.file.Truncate(0); err != nil {
		return err
	}
	_, err := w.file.Seek(0, 0)
	return err
}