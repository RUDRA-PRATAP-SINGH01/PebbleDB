package db

import "github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"

// Delete removes a key (tombstone).
//
// Durability semantics match Put; see Put for group-commit behaviour and Sync().
func (db *DB) Delete(key []byte) error {
	return db.writeRecord(wal.Record{Key: key, Value: nil, Tombstone: true})
}
