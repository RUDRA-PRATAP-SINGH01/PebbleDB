package db

import (
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/sstable"
)

type memLookupResult int

const (
	memLookupMiss memLookupResult = iota
	memLookupHit
	memLookupTombstone
)

// Get retrieves the value for a key. Returns ErrNotFound if key does not exist
// or is a tombstone.
func (db *DB) Get(key []byte) ([]byte, error) {
	db.mu.RLock()
	if db.closed {
		db.mu.RUnlock()
		return nil, ErrClosed
	}

	if val, res := lookupMemtable(db.active, key); res == memLookupHit {
		db.mu.RUnlock()
		return val, nil
	} else if res == memLookupTombstone {
		db.mu.RUnlock()
		return nil, ErrNotFound
	}

	for i := len(db.pendingFlush) - 1; i >= 0; i-- {
		if val, res := lookupMemtable(db.pendingFlush[i].mem, key); res == memLookupHit {
			db.mu.RUnlock()
			return val, nil
		} else if res == memLookupTombstone {
			db.mu.RUnlock()
			return nil, ErrNotFound
		}
	}

	readers := append([]*sstable.Reader(nil), db.sstables...)
	for _, r := range readers {
		r.Ref()
	}
	db.mu.RUnlock()
	defer func() {
		for _, r := range readers {
			r.Unref()
		}
	}()

	for i := len(readers) - 1; i >= 0; i-- {
		if !readers[i].MayContain(key) {
			continue
		}
		val, found, tomb, err := readers[i].Get(key)
		if err != nil {
			return nil, err
		}
		if found {
			if tomb {
				return nil, ErrNotFound
			}
			return val, nil
		}
	}
	return nil, ErrNotFound
}

func lookupMemtable(mt *memtable.SkipList, key []byte) ([]byte, memLookupResult) {
	if mt == nil {
		return nil, memLookupMiss
	}
	val, found, tombstone := mt.Get(key)
	if !found {
		return nil, memLookupMiss
	}
	if tombstone {
		return nil, memLookupTombstone
	}
	return val, memLookupHit
}
