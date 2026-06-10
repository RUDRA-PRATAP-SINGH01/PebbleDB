package sstable

import "bytes"

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
		blockData := make([]byte, entry.Length)
		if _, err := it.reader.file.ReadAt(blockData, int64(entry.Offset)); err != nil {
			it.err = err
			it.valid = false
			return
		}
		it.blockIt = NewBlockIterator(blockData)
	}
}

// Path returns the on-disk path of the SSTable.
func (r *Reader) Path() string {
	if r.file == nil {
		return ""
	}
	return r.file.Name()
}

// MergeReaders writes the union of readers to w. Readers must be ordered oldest
// to newest; newer tables win on duplicate keys and tombstones are dropped.
func MergeReaders(readers []*Reader, w *Writer) error {
	if len(readers) == 0 {
		return nil
	}

	iters := make([]*Iterator, len(readers))
	for i, r := range readers {
		iters[i] = r.NewIterator()
	}

	for {
		minKey, at := minKeyAcross(iters)
		if minKey == nil {
			break
		}

		winner := iters[at[len(at)-1]]
		if !winner.tombstone {
			if err := w.Add(winner.key, winner.value, false); err != nil {
				return err
			}
		}

		for _, idx := range at {
			if bytesEqual(iters[idx].key, minKey) {
				iters[idx].Next()
				if err := iters[idx].Err(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func minKeyAcross(iters []*Iterator) ([]byte, []int) {
	var minKey []byte
	var at []int
	for i, it := range iters {
		if !it.Valid() {
			continue
		}
		if minKey == nil || bytes.Compare(it.key, minKey) < 0 {
			minKey = it.key
			at = []int{i}
		} else if bytes.Compare(it.key, minKey) == 0 {
			at = append(at, i)
		}
	}
	return minKey, at
}

func bytesEqual(a, b []byte) bool {
	return bytes.Compare(a, b) == 0
}
