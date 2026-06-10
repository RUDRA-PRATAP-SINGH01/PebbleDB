package db

import "github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"

func (db *DB) writeRecord(rec wal.Record, apply func()) error {
	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return ErrClosed
	}
	if err := db.wal.Append(rec); err != nil {
		db.mu.Unlock()
		return err
	}
	if err := db.wal.Sync(); err != nil {
		db.mu.Unlock()
		return err
	}
	apply()
	shouldFlush, err := db.maybeFlushLocked()
	db.mu.Unlock()
	if err != nil {
		return err
	}
	if shouldFlush {
		db.flushCh <- struct{}{}
	}
	return nil
}
