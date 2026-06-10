package db

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/sstable"
)

func (db *DB) flusher() {
	for range db.flushCh {
		db.mu.Lock()
		if db.immutable == nil {
			db.mu.Unlock()
			continue
		}
		imm := db.immutable
		db.immutable = nil
		db.mu.Unlock()

		id := atomic.AddUint64(&db.nextSSTID, 1)
		path := filepath.Join(db.dir, fmt.Sprintf("sst_%08d.sst", id))
		w, err := sstable.NewWriter(path, defaultBlockSize)
		if err != nil {
			continue
		}

		it := imm.Iterator()
		flushErr := false
		for it.Valid() {
			if err := w.Add(it.Key(), it.Value(), it.IsTombstone()); err != nil {
				flushErr = true
				break
			}
			it.Next()
		}
		it.Close()

		if flushErr {
			w.Close()
			os.Remove(path)
			continue
		}
		if err := w.Close(); err != nil {
			os.Remove(path)
			continue
		}

		r, err := sstable.OpenReader(path)
		if err != nil {
			os.Remove(path)
			continue
		}

		db.mu.Lock()
		db.sstables = append(db.sstables, r)
		db.mu.Unlock()

		db.wal.Truncate()
	}
	close(db.flushDone)
}

func (db *DB) maybeFlushLocked() bool {
	if db.immutable != nil {
		return false
	}
	if db.active.Size() <= db.memtableThreshold {
		return false
	}
	db.immutable = db.active
	db.active = memtable.NewSkipList()
	return true
}
