package db

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/sstable"
)

const flushRetryDelay = 100 * time.Millisecond

func (db *DB) flusher() {
	for range db.flushCh {
		db.drainPendingFlush()
	}
	close(db.flushDone)
}

// drainPendingFlush processes every queued memtable per wakeup so dropped
// flush signals cannot leave entries stuck in pendingFlush.
func (db *DB) drainPendingFlush() {
	for {
		db.mu.Lock()
		if len(db.pendingFlush) == 0 {
			db.mu.Unlock()
			return
		}
		entry := db.pendingFlush[0]
		db.mu.Unlock()

		if err := db.flushImmutable(entry.mem, entry.walCutoff); err != nil {
			db.setBackgroundErr("flush", err)
			log.Printf("pebbledb: flush error: %v (retrying)", err)
			time.Sleep(flushRetryDelay)
			db.notifyFlush()
			return
		}

		db.mu.Lock()
		if len(db.pendingFlush) > 0 {
			db.pendingFlush = db.pendingFlush[1:]
		}
		db.mu.Unlock()
		db.clearBackgroundErr()
	}
}

func (db *DB) notifyFlush() {
	select {
	case db.flushCh <- struct{}{}:
	default:
		go func() { db.flushCh <- struct{}{} }()
	}
}

func (db *DB) flushImmutable(imm *memtable.SkipList, walCutoff int64) error {
	id := atomic.AddUint64(&db.nextSSTID, 1)
	path := filepath.Join(db.dir, fmt.Sprintf("sst_%08d.sst", id))

	expectedEntries := uint(imm.Len())
	if expectedEntries < 1 {
		expectedEntries = 1
	}
	w, err := sstable.NewWriter(path, defaultBlockSize, expectedEntries)
	if err != nil {
		return err
	}

	it := imm.Iterator()
	for it.Valid() {
		if err := w.Add(it.Key(), it.Value(), it.IsTombstone()); err != nil {
			it.Close()
			w.Close()
			os.Remove(path)
			return err
		}
		it.Next()
	}
	it.Close()

	if err := w.Close(); err != nil {
		os.Remove(path)
		return err
	}

	r, err := sstable.OpenReader(path)
	if err != nil {
		os.Remove(path)
		return err
	}

	if err := db.manifest.AppendNewFile(id); err != nil {
		r.Close()
		os.Remove(path)
		return err
	}

	if err := writeWalFlushState(db.dir, walFlushState{
		FreezeOffset: walCutoff,
		SSTID:        id,
	}); err != nil {
		r.Close()
		os.Remove(path)
		return err
	}

	if err := db.wal.TruncateBefore(walCutoff); err != nil {
		r.Close()
		os.Remove(path)
		return err
	}

	if err := removeWalFlushState(db.dir); err != nil {
		r.Close()
		return err
	}

	db.mu.Lock()
	db.sstables = append(db.sstables, r)
	db.mu.Unlock()
	db.maybeTriggerCompaction()
	return nil
}

func (db *DB) maybeFlushLocked() (bool, error) {
	if db.active.Size() <= db.memtableSize {
		return false, nil
	}
	offset, err := db.wal.Size()
	if err != nil {
		return false, err
	}
	db.pendingFlush = append(db.pendingFlush, flushQueueEntry{
		mem:       db.active,
		walCutoff: offset,
	})
	db.active = memtable.NewSkipList()
	return true, nil
}

func (db *DB) hasPendingFlush() bool {
	return len(db.pendingFlush) > 0
}
