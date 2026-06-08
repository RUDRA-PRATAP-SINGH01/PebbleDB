package db

import "errors"

var ErrNotFound = errors.New("key not found")
var ErrClosed = errors.New("database closed")

// Get retrieves the value for a key. Returns ErrNotFound if key does not exist
// or is a tombstone. Note: tombstones are not returned but are still present.
func (db *DB) Get(key []byte) ([]byte, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if db.closed {
		return nil, ErrClosed
	}
	val, found, tombstone := db.active.Get(key)
	if !found || tombstone {
		return nil, ErrNotFound
	}
	return val, nil
}