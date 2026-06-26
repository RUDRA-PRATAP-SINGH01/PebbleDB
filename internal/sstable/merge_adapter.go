package sstable

import "github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/iterator"

// iterAdapter adapts *Iterator to iterator.Iterator (Next returns I/O errors).
type iterAdapter struct {
	it *Iterator
}

func (a *iterAdapter) Valid() bool       { return a.it.Valid() }
func (a *iterAdapter) Key() []byte       { return a.it.Key() }
func (a *iterAdapter) Value() []byte     { return a.it.Value() }
func (a *iterAdapter) IsTombstone() bool { return a.it.IsTombstone() }

func (a *iterAdapter) Next() error {
	a.it.Next()
	return a.it.Err()
}

func (a *iterAdapter) Seek(key []byte) error {
	return a.it.Seek(key)
}

func (a *iterAdapter) Close() error {
	return a.it.Close()
}

func mergeReaders(readers []*Reader, w *Writer, keepTombstones bool) error {
	if len(readers) == 0 {
		return nil
	}

	adapters := make([]iterator.Iterator, len(readers))
	priorities := make([]int, len(readers))
	for i, r := range readers {
		adapters[i] = &iterAdapter{it: r.NewIterator()}
		priorities[i] = i
	}
	defer func() {
		for _, a := range adapters {
			a.Close()
		}
	}()

	return iterator.ForEachMerged(adapters, priorities, !keepTombstones, func(key, value []byte, tombstone bool) error {
		return w.Add(key, value, tombstone)
	})
}
