package sstable

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/bloom"
)

type Reader struct {
	file           *os.File
	path           string
	footer         Footer
	index          []IndexEntry
	bloom          *bloom.Filter
	fileID         uint64
	blockCache     *BlockCache
	refs           atomic.Int32
	closePending   atomic.Bool
	discardPending atomic.Bool
	fileClosed     atomic.Bool
	closeMu        sync.Mutex
}

// OpenReader opens an SSTable at path. cache may be nil to disable block caching.
func OpenReader(path string, cache *BlockCache) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	footer, err := ReadFooter(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	fileSize := fi.Size()
	dataEnd := fileSize - int64(footerSize)

	if int64(footer.IndexOffset) < 0 || int64(footer.IndexOffset+footer.IndexLength) > dataEnd {
		f.Close()
		return nil, ErrCorruptIndex
	}
	if footer.BloomLength > 0 {
		if int64(footer.BloomOffset) < 0 || int64(footer.BloomOffset+footer.BloomLength) > dataEnd {
			f.Close()
			return nil, ErrCorruptBloom
		}
		if footer.BloomOffset < footer.IndexOffset+footer.IndexLength {
			f.Close()
			return nil, ErrCorruptBloom
		}
	}
	if _, err := f.Seek(int64(footer.IndexOffset), io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	indexData := make([]byte, footer.IndexLength)
	if _, err := io.ReadFull(f, indexData); err != nil {
		f.Close()
		return nil, err
	}
	index, err := decodeIndex(indexData)
	if err != nil {
		f.Close()
		return nil, err
	}

	var bf *bloom.Filter
	if footer.BloomLength > 0 {
		bloomData := make([]byte, footer.BloomLength)
		if _, err := f.ReadAt(bloomData, int64(footer.BloomOffset)); err != nil {
			f.Close()
			return nil, err
		}
		bf = bloom.Decode(bloomData)
		if bf == nil {
			f.Close()
			return nil, ErrCorruptBloom
		}
	}

	return &Reader{
		file:       f,
		path:       f.Name(),
		footer:     *footer,
		index:      index,
		bloom:      bf,
		fileID:     nextReaderFileID.Add(1),
		blockCache: cache,
	}, nil
}

// readBlock returns a block at offset/length, consulting the LRU cache first.
func (r *Reader) readBlock(offset, length uint64) ([]byte, error) {
	if r.file == nil || r.fileClosed.Load() {
		return nil, os.ErrClosed
	}

	key := blockCacheKey(r.fileID, offset)
	if r.blockCache != nil {
		if data, ok := r.blockCache.get(key); ok {
			return data, nil
		}
	}

	blockData := make([]byte, length)
	if _, err := r.file.ReadAt(blockData, int64(offset)); err != nil {
		return nil, err
	}
	if r.blockCache != nil {
		r.blockCache.add(key, blockData)
	}
	return blockData, nil
}

func decodeIndex(data []byte) ([]IndexEntry, error) {
	var entries []IndexEntry
	pos := 0
	for pos < len(data) {
		if pos+4 > len(data) {
			return nil, ErrCorruptIndex
		}
		keyLen := binary.BigEndian.Uint32(data[pos:])
		pos += 4
		if pos+int(keyLen) > len(data) {
			return nil, ErrCorruptIndex
		}
		lastKey := data[pos : pos+int(keyLen)]
		pos += int(keyLen)
		if pos+16 > len(data) {
			return nil, ErrCorruptIndex
		}
		offset := binary.BigEndian.Uint64(data[pos:])
		pos += 8
		length := binary.BigEndian.Uint64(data[pos:])
		pos += 8
		entries = append(entries, IndexEntry{
			LastKey: append([]byte(nil), lastKey...),
			Offset:  offset,
			Length:  length,
		})
	}
	return entries, nil
}

// MayContain reports whether the key might be in this SSTable.
// Returns true if no bloom filter is present.
func (r *Reader) MayContain(key []byte) bool {
	if r.bloom == nil {
		return true
	}
	return r.bloom.MayContain(key)
}

func (r *Reader) Get(key []byte) (value []byte, found bool, tombstone bool, err error) {
	if !r.MayContain(key) {
		return nil, false, false, nil
	}
	idx := sort.Search(len(r.index), func(i int) bool {
		return bytes.Compare(key, r.index[i].LastKey) <= 0
	})
	if idx == len(r.index) {
		return nil, false, false, nil
	}
	entry := r.index[idx]
	blockData, err := r.readBlock(entry.Offset, entry.Length)
	if err != nil {
		return nil, false, false, err
	}
	it := NewBlockIterator(blockData)
	for it.Next() {
		cmp := bytes.Compare(key, it.Key())
		if cmp == 0 {
			return it.Value(), true, it.IsTombstone(), nil
		}
		if cmp < 0 {
			break
		}
	}
	return nil, false, false, nil
}

// Ref increments the number of in-flight readers of this SSTable.
func (r *Reader) Ref() {
	r.refs.Add(1)
}

// Unref decrements in-flight readers and closes the file when pending.
func (r *Reader) Unref() {
	if r.refs.Add(-1) == 0 && r.closePending.Load() {
		r.closeFile()
	}
}

// Close marks the reader closed; the file is closed once all refs are released.
func (r *Reader) Close() error {
	r.closePending.Store(true)
	if r.refs.Load() == 0 {
		return r.closeFile()
	}
	return nil
}

// Discard marks the reader for removal and deletes the backing file once all
// in-flight Ref holders have called Unref. Only call after the reader is
// removed from the live SSTable set.
func (r *Reader) Discard() error {
	r.closePending.Store(true)
	r.discardPending.Store(true)
	if r.refs.Load() == 0 {
		return r.closeFile()
	}
	return nil
}

func (r *Reader) closeFile() error {
	r.closeMu.Lock()
	defer r.closeMu.Unlock()
	if r.fileClosed.Load() || r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	r.fileClosed.Store(true)
	if r.discardPending.Load() && r.path != "" {
		_ = os.Remove(r.path)
	}
	return err
}

// EntryCount returns the number of entries in the SSTable.
func (r *Reader) EntryCount() (uint, error) {
	it := r.NewIterator()
	defer it.Close()
	var n uint
	for it.Valid() {
		n++
		it.Next()
		if err := it.Err(); err != nil {
			return 0, err
		}
	}
	return n, nil
}
