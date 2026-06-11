package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/sstable"
)

const (
	defaultCompactThreshold = 4
	defaultCompactPickCount = 2
)

func (db *DB) maybeTriggerCompaction() {
	db.mu.RLock()
	closed := db.closed
	threshold := db.compactThreshold
	count := len(db.sstables)
	db.mu.RUnlock()

	if closed || threshold <= 0 || count < threshold {
		return
	}
	select {
	case db.compactCh <- struct{}{}:
	default:
	}
}

// pickSSTablesForCompactionLocked returns the oldest SSTables to merge.
// db.mu must be held.
func (db *DB) pickSSTablesForCompactionLocked() []*sstable.Reader {
	if len(db.sstables) < db.compactThreshold {
		return nil
	}
	n := defaultCompactPickCount
	if len(db.sstables) < n {
		n = len(db.sstables)
	}
	if n < 2 {
		return nil
	}
	picked := make([]*sstable.Reader, n)
	copy(picked, db.sstables[:n])
	return picked
}

func readersStillPresent(all []*sstable.Reader, subset []*sstable.Reader) bool {
	for _, want := range subset {
		found := false
		for _, r := range all {
			if r == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (db *DB) mergeSSTables(readers []*sstable.Reader) (*sstable.Reader, uint64, error) {
	if len(readers) < 2 {
		return nil, 0, nil
	}

	var expectedEntries uint
	for _, r := range readers {
		n, err := r.EntryCount()
		if err != nil {
			return nil, 0, err
		}
		expectedEntries += n
	}
	if expectedEntries < 1 {
		expectedEntries = 1
	}

	id := atomic.AddUint64(&db.nextSSTID, 1)
	path := filepath.Join(db.dir, fmt.Sprintf("sst_%08d.sst", id))

	w, err := sstable.NewWriter(path, defaultBlockSize, expectedEntries)
	if err != nil {
		return nil, 0, err
	}

	if err := sstable.MergeReadersKeepTombstones(readers, w); err != nil {
		w.Close()
		os.Remove(path)
		return nil, 0, err
	}
	if err := w.Close(); err != nil {
		os.Remove(path)
		return nil, 0, err
	}

	merged, err := sstable.OpenReader(path)
	if err != nil {
		os.Remove(path)
		return nil, 0, err
	}
	return merged, id, nil
}

func sstIDFromPath(path string) (uint64, error) {
	base := filepath.Base(path)
	m := sstFilePattern.FindStringSubmatch(base)
	if m == nil {
		return 0, fmt.Errorf("invalid sstable path %q", path)
	}
	id, err := strconv.ParseUint(m[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid sstable path %q: %w", path, err)
	}
	return id, nil
}

func liveIDsFromReaders(readers []*sstable.Reader) ([]uint64, error) {
	ids := make([]uint64, 0, len(readers))
	for _, r := range readers {
		id, err := sstIDFromPath(r.Path())
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
