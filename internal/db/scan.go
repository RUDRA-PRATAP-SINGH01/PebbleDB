package db

import (
	"bytes"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/iterator"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/sstable"
)

const (
	scanPriorityActive    = 1_000_000
	scanPriorityImmutable = 999_999
)

// ScanIterator walks keys in [start, end) across memtables and SSTables.
type ScanIterator struct {
	db    *DB
	merge *iterator.MergeIterator
	end   []byte
	readers []*sstable.Reader
	closed  bool
}

func (it *ScanIterator) Valid() bool {
	if it.closed || it.db == nil {
		return false
	}
	it.db.mu.RLock()
	closed := it.db.closed
	it.db.mu.RUnlock()
	if closed {
		return false
	}
	if it.merge == nil || !it.merge.Valid() {
		return false
	}
	if len(it.end) == 0 {
		return true
	}
	return bytes.Compare(it.merge.Key(), it.end) < 0
}

func (it *ScanIterator) Key() []byte   { return it.merge.Key() }
func (it *ScanIterator) Value() []byte { return it.merge.Value() }

func (it *ScanIterator) IsTombstone() bool { return false }

func (it *ScanIterator) Next() error {
	if it.closed {
		return ErrClosed
	}
	it.db.mu.RLock()
	closed := it.db.closed
	it.db.mu.RUnlock()
	if closed {
		return ErrClosed
	}
	if err := it.merge.Next(); err != nil {
		return err
	}
	return nil
}

func (it *ScanIterator) Seek(key []byte) error {
	if it.closed {
		return ErrClosed
	}
	it.db.mu.RLock()
	closed := it.db.closed
	it.db.mu.RUnlock()
	if closed {
		return ErrClosed
	}
	return it.merge.Seek(key)
}

func (it *ScanIterator) Close() error {
	if it.closed {
		return nil
	}
	it.closed = true
	var err error
	if it.merge != nil {
		err = it.merge.Close()
	}
	for _, r := range it.readers {
		r.Unref()
	}
	it.readers = nil
	return err
}

// Scan returns an iterator over keys in the half-open range [start, end).
// A nil or empty end scans to the last key.
func (db *DB) Scan(start, end []byte) (*ScanIterator, error) {
	if err := db.backgroundErr(); err != nil {
		return nil, err
	}

	db.mu.RLock()
	if db.closed {
		db.mu.RUnlock()
		return nil, ErrClosed
	}

	var sources []iterator.Iterator
	var priorities []int

	activeIt := &memtableIter{it: db.active.Iterator()}
	sources = append(sources, activeIt)
	priorities = append(priorities, scanPriorityActive)

	for i := len(db.pendingFlush) - 1; i >= 0; i-- {
		immIt := &memtableIter{it: db.pendingFlush[i].mem.Iterator()}
		sources = append(sources, immIt)
		pri := scanPriorityImmutable - (len(db.pendingFlush) - 1 - i)
		priorities = append(priorities, pri)
	}

	readers := append([]*sstable.Reader(nil), db.sstables...)
	for _, r := range readers {
		r.Ref()
	}
	for i, r := range readers {
		sources = append(sources, &sstableIter{it: r.NewIterator()})
		priorities = append(priorities, i)
	}
	db.mu.RUnlock()

	merge, err := iterator.NewMergeIterator(sources, priorities)
	if err != nil {
		closeScanSources(sources)
		for _, r := range readers {
			r.Unref()
		}
		return nil, err
	}
	if err := merge.Seek(start); err != nil {
		merge.Close()
		for _, r := range readers {
			r.Unref()
		}
		return nil, err
	}

	return &ScanIterator{
		db:      db,
		merge:   merge,
		end:     append([]byte(nil), end...),
		readers: readers,
	}, nil
}

func closeScanSources(sources []iterator.Iterator) {
	for _, s := range sources {
		s.Close()
	}
}

type memtableIter struct {
	it *memtable.SkipListIterator
}

func (m *memtableIter) Valid() bool       { return m.it.Valid() }
func (m *memtableIter) Key() []byte       { return m.it.Key() }
func (m *memtableIter) Value() []byte     { return m.it.Value() }
func (m *memtableIter) IsTombstone() bool { return m.it.IsTombstone() }

func (m *memtableIter) Next() error {
	m.it.Next()
	return nil
}

func (m *memtableIter) Seek(key []byte) error {
	m.it.Seek(key)
	return nil
}

func (m *memtableIter) Close() error {
	m.it.Close()
	return nil
}

type sstableIter struct {
	it *sstable.Iterator
}

func (s *sstableIter) Valid() bool       { return s.it.Valid() }
func (s *sstableIter) Key() []byte       { return s.it.Key() }
func (s *sstableIter) Value() []byte     { return s.it.Value() }
func (s *sstableIter) IsTombstone() bool { return s.it.IsTombstone() }

func (s *sstableIter) Next() error {
	s.it.Next()
	return s.it.Err()
}

func (s *sstableIter) Seek(key []byte) error {
	return s.it.Seek(key)
}

func (s *sstableIter) Close() error {
	return s.it.Close()
}
