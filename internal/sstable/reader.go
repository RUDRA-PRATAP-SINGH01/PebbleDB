package sstable

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"sort"
)

type Reader struct {
	file   *os.File
	footer Footer
	index  []IndexEntry
}

func OpenReader(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	footer, err := ReadFooter(f)
	if err != nil {
		f.Close()
		return nil, err
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
	return &Reader{file: f, footer: *footer, index: index}, nil
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

func (r *Reader) Get(key []byte) (value []byte, found bool, tombstone bool, err error) {
	idx := sort.Search(len(r.index), func(i int) bool {
		return bytes.Compare(key, r.index[i].LastKey) <= 0
	})
	if idx == len(r.index) {
		return nil, false, false, nil
	}
	entry := r.index[idx]
	blockData := make([]byte, entry.Length)
	if _, err := r.file.ReadAt(blockData, int64(entry.Offset)); err != nil {
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

func (r *Reader) Close() error {
	return r.file.Close()
}
