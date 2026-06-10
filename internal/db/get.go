package db

import (
	"errors"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
)

var ErrNotFound = errors.New("key not found")
var ErrClosed = errors.New("database closed")

// Get retrieves the value for a key. Returns ErrNotFound if key does not exist
// or is a tombstone.
func (db *DB) Get(key []byte) ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return nil, ErrClosed
	}

	if val, ok := db.getFromMemtable(db.active, key); ok {
		return val, nil
	}
	if val, ok := db.getFromMemtable(db.immutable, key); ok {
		return val, nil
	}

	// Search SSTables newest to oldest for the latest value.
	for i := len(db.sstables) - 1; i >= 0; i-- {
		val, found, tomb, err := db.sstables[i].Get(key)
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

func (db *DB) getFromMemtable(mt *memtable.SkipList, key []byte) ([]byte, bool) {
	if mt == nil {
		return nil, false
	}
	val, found, tombstone := mt.Get(key)
	if !found || tombstone {
		return nil, false
	}
	return val, true
}
