package sstable

import (
	"bytes"
	"sort"
)

// Iterator walks all entries in an SSTable in ascending key order.
type Iterator struct {
	reader    *Reader
	blockIdx  int
	blockIt   *BlockIterator
	key       []byte
	value     []byte
	tombstone bool
	valid     bool
	err       error
}

// NewIterator returns an iterator positioned at the first entry.
func (r *Reader) NewIterator() *Iterator {
	it := &Iterator{reader: r, blockIdx: -1}
	it.advance()
	return it
}

// Valid reports whether the iterator points at a readable entry.
func (it *Iterator) Valid() bool {
	return it.valid
}

// Key returns a copy of the current key.
func (it *Iterator) Key() []byte {
	if it.key == nil {
		return nil
	}
	return append([]byte(nil), it.key...)
}

// Value returns a copy of the current value.
func (it *Iterator) Value() []byte {
	if it.value == nil {
		return nil
	}
	return append([]byte(nil), it.value...)
}

// IsTombstone reports whether the current entry is a tombstone.
func (it *Iterator) IsTombstone() bool {
	return it.tombstone
}

// Err returns the first I/O error encountered while advancing.
func (it *Iterator) Err() error {
	return it.err
}

// Next advances to the following entry.
func (it *Iterator) Next() {
	if !it.valid {
		return
	}
	it.advance()
}

// Seek positions at the first entry with key >= target.
func (it *Iterator) Seek(key []byte) error {
	it.err = nil
	if len(it.reader.index) == 0 {
		it.valid = false
		return nil
	}
	if len(key) == 0 {
		it.blockIdx = -1
		it.blockIt = nil
		it.advance()
		return it.err
	}
	idx := sort.Search(len(it.reader.index), func(i int) bool {
		return bytes.Compare(key, it.reader.index[i].LastKey) <= 0
	})
	if idx >= len(it.reader.index) {
		it.valid = false
		return nil
	}
	for bi := idx; bi < len(it.reader.index); bi++ {
		entry := it.reader.index[bi]
		blockData, err := it.reader.readBlock(entry.Offset, entry.Length)
		if err != nil {
			it.err = err
			it.valid = false
			return err
		}
		blockIt := NewBlockIterator(blockData)
		if blockIt.Seek(key) {
			it.blockIdx = bi
			it.blockIt = blockIt
			it.key = blockIt.Key()
			it.value = blockIt.Value()
			it.tombstone = blockIt.IsTombstone()
			it.valid = true
			return nil
		}
	}
	it.valid = false
	return nil
}

// Close releases iterator resources (no-op for SSTable iterators).
func (it *Iterator) Close() error {
	it.valid = false
	return nil
}

func (it *Iterator) advance() {
	for {
		if it.blockIt != nil && it.blockIt.Next() {
			it.key = it.blockIt.Key()
			it.value = it.blockIt.Value()
			it.tombstone = it.blockIt.IsTombstone()
			it.valid = true
			return
		}

		it.blockIdx++
		if it.blockIdx >= len(it.reader.index) {
			it.valid = false
			return
		}

		entry := it.reader.index[it.blockIdx]
		blockData, err := it.reader.readBlock(entry.Offset, entry.Length)
		if err != nil {
			it.err = err
			it.valid = false
			return
		}
		it.blockIt = NewBlockIterator(blockData)
	}
}

// Path returns the on-disk path of the SSTable.
func (r *Reader) Path() string {
	return r.path
}

// MergeReaders writes the union of readers to w. Readers must be ordered oldest
// to newest; newer tables win on duplicate keys and tombstones are dropped.
func MergeReaders(readers []*Reader, w *Writer) error {
	return mergeReaders(readers, w, false)
}

// MergeReadersKeepTombstones merges readers and preserves tombstone entries.
func MergeReadersKeepTombstones(readers []*Reader, w *Writer) error {
	return mergeReaders(readers, w, true)
}
