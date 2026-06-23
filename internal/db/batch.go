package db

import (
	"bytes"
	"sort"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"
)

const (
	batchMaxRecords = 64
	batchMaxBytes   = 16 * 1024
	batchFlushDelay = 1 * time.Millisecond
)

func recordWireSize(rec wal.Record) int {
	return 4 + len(rec.Key) + 4 + len(rec.Value) + 1 + 4
}

// ownedRecord copies key/value once when the record is queued for WAL batching.
func ownedRecord(rec wal.Record) wal.Record {
	out := wal.Record{Tombstone: rec.Tombstone}
	if rec.Key != nil {
		out.Key = append([]byte(nil), rec.Key...)
	}
	if rec.Value != nil {
		out.Value = append([]byte(nil), rec.Value...)
	}
	return out
}

func applyRecordToMemtable(db *DB, rec wal.Record) {
	if rec.Tombstone {
		db.active.Delete(rec.Key)
	} else {
		db.active.Put(rec.Key, rec.Value)
	}
}

// takePendingBatchLocked moves the current WAL batch out of the queue without copying.
// db.mu must be held.
func takePendingBatchLocked(db *DB) []wal.Record {
	if len(db.pendingBatch) == 0 {
		return nil
	}
	batch := db.pendingBatch
	db.pendingBatch = nil
	db.batchSizeBytes = 0
	return batch
}

// restorePendingBatchLocked prepends a failed WAL batch back onto the queue.
// db.mu must be held.
func restorePendingBatchLocked(db *DB, batch []wal.Record) {
	db.pendingBatch = append(batch, db.pendingBatch...)
	for _, rec := range batch {
		db.batchSizeBytes += recordWireSize(rec)
	}
}

func (db *DB) scheduleBatchFlushLocked() {
	if db.batchTimer == nil {
		db.batchTimer = time.AfterFunc(batchFlushDelay, func() {
			select {
			case db.batchFlushCh <- struct{}{}:
			default:
			}
		})
		return
	}
	db.batchTimer.Reset(batchFlushDelay)
}

func (db *DB) batchFlusher() {
	defer close(db.batchDone)

	for {
		select {
		case <-db.batchStop:
			_ = db.flushPendingBatch()
			return
		case reply := <-db.batchSyncCh:
			reply <- db.flushPendingBatch()
		case <-db.batchFlushCh:
			_ = db.flushPendingBatch()
		}
	}
}

// awaitBatchPersist hands WAL persistence to the batch flusher goroutine.
func (db *DB) awaitBatchPersist() error {
	reply := make(chan error, 1)
	db.batchSyncCh <- reply
	return <-reply
}

// flushPendingBatch writes the in-memory batch to the WAL (one fsync) and applies
// records to the active memtable. Safe to call with an empty batch.
func (db *DB) flushPendingBatch() error {
	db.mu.Lock()
	if len(db.pendingBatch) == 0 {
		db.mu.Unlock()
		return nil
	}
	batch := takePendingBatchLocked(db)
	db.mu.Unlock()

	if err := db.wal.AppendBatch(batch); err != nil {
		db.mu.Lock()
		restorePendingBatchLocked(db, batch)
		db.mu.Unlock()
		db.setBackgroundErr("wal", err)
		return err
	}

	db.mu.Lock()
	for _, rec := range batch {
		applyRecordToMemtable(db, rec)
	}
	shouldFlush, err := db.maybeFlushLocked()
	// Reuse the flushed slice only when no new records were queued during WAL append.
	// Otherwise we would drop writes that landed in pendingBatch while fsync ran.
	if len(db.pendingBatch) == 0 {
		db.pendingBatch = batch[:0]
	}
	db.mu.Unlock()
	if err != nil {
		return err
	}
	if shouldFlush {
		db.notifyFlush()
	}
	return nil
}

func (db *DB) stopBatchFlusher() {
	if db.batchStop == nil {
		return
	}
	close(db.batchStop)
	<-db.batchDone
	db.batchStop = nil
}

func lookupPendingBatch(batch []wal.Record, key []byte) ([]byte, memLookupResult) {
	for i := len(batch) - 1; i >= 0; i-- {
		rec := batch[i]
		if !bytes.Equal(rec.Key, key) {
			continue
		}
		if rec.Tombstone {
			return nil, memLookupTombstone
		}
		return append([]byte(nil), rec.Value...), memLookupHit
	}
	return nil, memLookupMiss
}

func snapshotPendingBatch(batch []wal.Record) []memtable.SnapshotEntry {
	if len(batch) == 0 {
		return nil
	}
	type latest struct {
		rec wal.Record
		ord int
	}
	byKey := make(map[string]latest, len(batch))
	for i, rec := range batch {
		byKey[string(rec.Key)] = latest{rec: rec, ord: i}
	}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]memtable.SnapshotEntry, len(keys))
	for i, k := range keys {
		rec := byKey[k].rec
		e := memtable.SnapshotEntry{
			Key:       append([]byte(nil), rec.Key...),
			Tombstone: rec.Tombstone,
		}
		if !rec.Tombstone {
			e.Value = append([]byte(nil), rec.Value...)
		}
		out[i] = e
	}
	return out
}
