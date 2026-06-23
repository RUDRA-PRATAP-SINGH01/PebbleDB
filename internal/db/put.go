package db

import "github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"

// Put writes a key-value pair into the database.
//
// Durability: unless Options.SyncWrites is true, Put may return before the WAL
// is fsynced (group commit). Call Sync() after writes that must survive crash, or
// enable SyncWrites at open time.
func (db *DB) Put(key, value []byte) error {
	return db.writeRecord(wal.Record{Key: key, Value: value, Tombstone: false})
}
