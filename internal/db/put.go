package db

import "github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"

// Put writes a key-value pair into the database.
func (db *DB) Put(key, value []byte) error {
	return db.writeRecord(wal.Record{Key: key, Value: value, Tombstone: false})
}
