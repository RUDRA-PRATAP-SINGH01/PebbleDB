package db

import (
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
)

// Close flushes pending data and closes the database.
func (db *DB) Close() error {
	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return nil
	}
	db.closed = true
	db.mu.Unlock()

	for {
		var needFlush bool
		db.mu.Lock()
		if db.immutable == nil && db.active.Size() <= 0 {
			db.mu.Unlock()
			break
		}
		if db.immutable == nil && db.active.Size() > 0 {
			offset, err := db.wal.Size()
			if err != nil {
				db.mu.Unlock()
				return err
			}
			db.walFreezeOffset = offset
			db.immutable = db.active
			db.active = memtable.NewSkipList()
			needFlush = true
		}
		db.mu.Unlock()

		if needFlush {
			select {
			case db.flushCh <- struct{}{}:
			default:
			}
		}
		db.waitForImmutableDrain()
	}

	close(db.flushCh)
	<-db.flushDone

	db.mu.Lock()
	defer db.mu.Unlock()
	for _, r := range db.sstables {
		r.Close()
	}
	db.sstables = nil
	if db.wal != nil {
		db.wal.Sync()
		db.wal.Close()
		db.wal = nil
	}
	if db.manifest != nil {
		db.manifest.Close()
		db.manifest = nil
	}
	return nil
}

func (db *DB) waitForImmutableDrain() {
	for {
		db.mu.RLock()
		imm := db.immutable
		db.mu.RUnlock()
		if imm == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}
