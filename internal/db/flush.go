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

			db.mu.RLock()
			closed := db.closed
			db.mu.RUnlock()
			if closed {
				return
			}

			time.Sleep(flushRetryDelay)
			continue
		}

		db.mu.Lock()
		if len(db.pendingFlush) > 0 {
			db.pendingFlush = db.pendingFlush[1:]
		}
		db.mu.Unlock()
		db.clearBackgroundErrOp("flush")
	}
}

func (db *DB) notifyFlush() {
	db.signalFlush(false)
}

// notifyFlushForce wakes the flusher during Close after db.closed is set.
func (db *DB) notifyFlushForce() {
	db.signalFlush(true)
}

func (db *DB) signalFlush(force bool) {
	if !force {
		db.mu.RLock()
		closed := db.closed
		db.mu.RUnlock()
		if closed {
			return
		}
	}
	db.flushCh <- struct{}{}
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
	maybeCrash(CrashAfterSSTClose)

	r, err := sstable.OpenReader(path, db.blockCache)
	if err != nil {
		os.Remove(path)
		return err
	}

	// Manifest commit is the durability boundary. After this point the SST must
	// remain on disk and become visible even if WAL cleanup or manifest rotation fails.
	if err := db.manifest.AppendNewFile(id); err != nil {
		r.Close()
		os.Remove(path)
		return err
	}
	maybeCrash(CrashAfterManifestNewFile)

	db.mu.Lock()
	db.sstables = append(db.sstables, r)
	db.mu.Unlock()
	db.trackReader(r)

	if err := db.manifest.MaybeCompact(); err != nil {
		log.Printf("pebbledb: manifest compaction after flush: %v (data is durable)", err)
	}

	if err := db.completeWalAfterFlush(walCutoff, id); err != nil {
		log.Printf("pebbledb: wal cleanup after flush of sst %d: %v (data is durable; reopen will recover)", id, err)
	} else {
		maybeCrash(CrashAfterWalTruncate)
	}

	db.maybeTriggerCompaction()
	return nil
}

func (db *DB) completeWalAfterFlush(walCutoff int64, sstID uint64) error {
	if err := writeWalFlushState(db.dir, walFlushState{
		FreezeOffset: walCutoff,
		SSTID:        sstID,
	}); err != nil {
		return err
	}
	maybeCrash(CrashAfterWalFlushState)
	if err := db.wal.TruncateBefore(walCutoff); err != nil {
		return err
	}
	return removeWalFlushState(db.dir)
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
