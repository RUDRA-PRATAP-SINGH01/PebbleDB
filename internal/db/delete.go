package db

import "github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"

// Delete removes a key (tombstone).
func (db *DB) Delete(key []byte) error {
	return db.writeRecord(
		wal.Record{Key: key, Value: nil, Tombstone: true},
		func() { db.active.Delete(key) },
	)
}
