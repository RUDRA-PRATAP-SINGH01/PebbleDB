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
		walCutoff := db.walFreezeOffset
		db.immutable = nil
		db.walFreezeOffset = 0
		db.mu.Unlock()

		if err := db.flushImmutable(imm, walCutoff); err != nil {
			db.setBackgroundErr("flush", err)
			db.restoreFailedFlush(imm, walCutoff)
			select {
			case db.flushCh <- struct{}{}:
			default:
			}
			continue
		}
		db.clearBackgroundErr()
		db.maybeCompact()
	}
	close(db.flushDone)
}

func (db *DB) flushImmutable(imm *memtable.SkipList, walCutoff int64) error {
	id := atomic.AddUint64(&db.nextSSTID, 1)
	path := filepath.Join(db.dir, fmt.Sprintf("sst_%08d.sst", id))

	expectedEntries := uint(imm.Len())
	if expectedEntries < 1 {
		expectedEntries = 1
	}
	w, err := sstable.NewWriterWithBloom(path, defaultBlockSize, expectedEntries)
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

	if err := db.wal.TruncateBefore(walCutoff); err != nil {
		r.Close()
		os.Remove(path)
		return err
	}

	db.mu.Lock()
	db.sstables = append(db.sstables, r)
	db.mu.Unlock()
	return nil
}

func (db *DB) restoreFailedFlush(imm *memtable.SkipList, walCutoff int64) {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.immutable == nil {
		db.immutable = imm
		db.walFreezeOffset = walCutoff
		return
	}

	it := imm.Iterator()
	for it.Valid() {
		if it.IsTombstone() {
			db.immutable.Delete(it.Key())
		} else {
			db.immutable.Put(it.Key(), it.Value())
		}
		it.Next()
	}
	it.Close()
}

func (db *DB) maybeFlushLocked() (bool, error) {
	if db.immutable != nil {
		return false, nil
	}
	if db.active.Size() <= db.memtableThreshold {
		return false, nil
	}
	offset, err := db.wal.Size()
	if err != nil {
		return false, err
	}
	db.walFreezeOffset = offset
	db.immutable = db.active
	db.active = memtable.NewSkipList()
	return true, nil
}
