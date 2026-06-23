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
	db      *DB
	merge   *iterator.MergeIterator
	end     []byte
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
	return it.merge.Next()
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
//
// The iterator is a point-in-time snapshot captured when Scan returns. Memtable
// layers are copied via Snapshot() and SSTable iterators are pinned at creation
// time, so writes and flushes that occur after Scan are not visible. Tombstones
// are omitted from iteration. Use Get for a live read of a single key, or call
// Scan again to observe newer data.
func (db *DB) Scan(start, end []byte) (*ScanIterator, error) {
	if err := db.blockingBackgroundErr(); err != nil {
		return nil, err
	}
	if err := db.flushPendingBatch(); err != nil {
		return nil, err
	}

	db.mu.RLock()
	if db.closed {
		db.mu.RUnlock()
		return nil, ErrClosed
	}

	var sources []iterator.Iterator
	var priorities []int

	activeSnap := db.active.Snapshot()
	sources = append(sources, newSnapshotMemIter(activeSnap))
	priorities = append(priorities, scanPriorityActive)

	for i := len(db.pendingFlush) - 1; i >= 0; i-- {
		snap := db.pendingFlush[i].mem.Snapshot()
		sources = append(sources, newSnapshotMemIter(snap))
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

// snapshotMemIter iterates a memtable.Snapshot without holding any lock.
type snapshotMemIter struct {
	entries []memtable.SnapshotEntry
	idx     int
}

func newSnapshotMemIter(entries []memtable.SnapshotEntry) *snapshotMemIter {
	return &snapshotMemIter{entries: entries}
}

func (s *snapshotMemIter) Valid() bool {
	return s.idx >= 0 && s.idx < len(s.entries)
}

func (s *snapshotMemIter) Key() []byte {
	if !s.Valid() {
		return nil
	}
	return append([]byte(nil), s.entries[s.idx].Key...)
}

func (s *snapshotMemIter) Value() []byte {
	if !s.Valid() {
		return nil
	}
	if s.entries[s.idx].Tombstone {
		return nil
	}
	return append([]byte(nil), s.entries[s.idx].Value...)
}

func (s *snapshotMemIter) IsTombstone() bool {
	if !s.Valid() {
		return false
	}
	return s.entries[s.idx].Tombstone
}

func (s *snapshotMemIter) Next() error {
	if s.idx < len(s.entries) {
		s.idx++
	}
	return nil
}

func (s *snapshotMemIter) Seek(key []byte) error {
	s.idx = 0
	for s.idx < len(s.entries) && bytes.Compare(s.entries[s.idx].Key, key) < 0 {
		s.idx++
	}
	return nil
}

func (s *snapshotMemIter) Close() error {
	s.entries = nil
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
