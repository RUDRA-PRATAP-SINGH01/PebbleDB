# System invariants

Properties PebbleDB must maintain across crash, concurrent reads, and background compaction. Grouped by concern: **durability** (power loss), **visibility** (caller observations), **concurrency** (parallel goroutines).

---

## Durability and authority

### Invariant D1 — Acknowledged writes are in the WAL before memtable apply

After `awaitBatchPersist()` or `Sync()` returns successfully, every record in the flushed batch has been appended to `wal.log` and `fsync` has completed. Memtable apply happens only after WAL append succeeds.

If memtable were updated before WAL fsync, a crash would lose data the client believed was written.

**Violation symptom.** Keys visible in memtable after restart but absent from WAL replay.

**Enforced by.** `flushPendingBatch()` in `internal/db/batch.go`: `AppendBatch` then apply loop. `restorePendingBatchLocked` on WAL failure.

**Crash boundary.** Between WAL append and memtable apply: safe — replay reconstructs memtable on open.

---

### Invariant D2 — A user-visible key exists in WAL or a live SST (or both)

For any key that `Get` would return (non-tombstone) at a logical point in time after successful `Sync()` or `Close()`:

- The key appears in `wal.log` at some offset, **or**
- The key appears in an SSTable whose id is in the manifest live set, **or**
- The key is in `active` / `pendingFlush` memtables that will be flushed before shutdown completes.

Before `Sync()`, async group commit may leave keys only in `pendingBatch` (not yet durable). That is an explicit API contract, not a violation.

This is the definition of durability for an LSM: the log plus immutable files are the source of truth.

**Violation symptom.** `Get` succeeds, crash, reopen → `ErrNotFound` with no WAL record and no SST entry.

**Enforced by.** WAL-before-memtable ordering; manifest-before-memory on flush/compaction; `Close` drains pending flush.

---

### Invariant D3 — Manifest is authoritative for the live SST set

An `sst_XXXXXXXX.sst` file on disk is **live** if and only if its id is in the manifest live set after replay. Directory glob, file mtime, or in-memory `db.sstables` alone do not define liveness.

Orphan SSTs after compaction crash confused recovery when discovery used directory glob.

**Violation symptom.** Disk has SST files not in manifest; `Get` misses keys that exist only in orphans. Or manifest lists ids with missing files → open error.

**Enforced by.** `loadSSTables()` loads only manifest ids; `removeOrphanSSTFiles()` quarantines extras; compaction uses `AppendSetFileSet` before swapping memory.

**Crash boundary.** Manifest record fsync is the moment a new SST becomes durable and live.

---

### Invariant D4 — Manifest fsync precedes in-memory SST set update (flush and compaction)

For flush: `manifest.AppendNewFile(id)` + fsync completes before `db.sstables` is updated.

For compaction: `manifest.AppendSetFileSet(liveIDs)` + fsync completes before `db.sstables` is replaced.

Memory-first ordering created windows where the process believed old SSTs were gone but manifest still listed them — or the reverse.

**Violation symptom.** Post-crash manifest and disk disagree; keys lost or duplicated across layers.

**Enforced by.** `flushImmutable`, `doCompaction` in `internal/db/flush.go` and `compactor.go`.

**Rollback.** If compaction picks readers that are invalidated before manifest commit, manifest rolls back to `oldLiveIDs` and merged file is deleted.

---

### Invariant D5 — WAL bytes before `FreezeOffset` are redundant with a flushed SST

When `wal.flush` exists and `walReplayStartOffset()` returns `FreezeOffset`:

- SST `SSTID` from the checkpoint is in the manifest live set.
- `wal.log` size ≥ `FreezeOffset`.
- All user records in WAL `[0, FreezeOffset)` are represented in that SST (or superseded by tombstones in that SST).

After successful truncate, `wal.flush` is removed and replay starts at offset 0 on the shorter file.

Defines the legal WAL replay range after restart.

**Violation symptom.** Duplicate keys in memtable + SST; stale values shadowing newer SST data.

**Enforced by.** Flush ordering: manifest commit → `writeWalFlushState` → `TruncateBefore` → `removeWalFlushState`. Open logic in `wal_state.go`.

**Crash windows.**

| Crash after | On reopen |
|-------------|-----------|
| Manifest commit, before `wal.flush` | Full WAL replay; SST also has data — duplicates possible in memtable until read path merges correctly; replay offset 0 |
| `wal.flush` written, before truncate | Replay from `FreezeOffset`; correct |
| Truncate done, before remove `wal.flush` | `wal.size < FreezeOffset` → replay from 0; correct |
| Truncate + remove `wal.flush` | Replay from 0 on short WAL; correct |

---

### Invariant D6 — SST files are complete before manifest learns them

An SST is written to a temp path pattern (`sst_%08d.sst` via writer), fully closed (footer + bloom), opened as `Reader`, then manifest is updated. Partial files are not appended to manifest.

Crash mid-write must not produce a live corrupt SST.

**Violation symptom.** Manifest references id; file truncated or bad footer → open fails.

**Enforced by.** `sstable.Writer` writes to path only after successful `Close`; flush removes file on manifest failure.

---

## Visibility and read path

### Invariant V1 — Get observes newest visible version across layers

Search order: `pendingBatch` (newest record per key) → `active` memtable → `pendingFlush` memtables (newest first) → SST readers (newest first). Tombstones mask older values.

Defines linearizability of reads relative to completed writes.

**Violation symptom.** `Get` returns deleted value; older SST shadows newer memtable entry.

**Enforced by.** `get.go` ordering; tombstone byte in WAL and SST blocks.

---

### Invariant V2 — Compaction does not close a reader still referenced by an in-flight Get or Scan

Compaction calls `Discard()` on input readers; physical `Close` happens only when `Ref()` count reaches zero.

Immutable SSTs are still readable while a goroutine holds a ref.

**Violation symptom.** Race detector failure; `os.ErrClosed` during block read; Windows delete while handle open.

**Enforced by.** `Ref`/`Unref` in `get.go` and `scan.go`; `readersStillPresent` in compaction. [compaction-race](../postmortems/compaction-race.md), [reader-lifecycle](../postmortems/reader-lifecycle.md).

---

### Invariant V3 — Scan iterators see a snapshot at creation time

`Scan` copies memtable state under lock (active + pending batch snapshot + pending flush list) and pins SST readers with `Ref`. Writes after iterator creation are not required to appear in the scan.

Avoids holding `db.mu` for the entire scan; defines iterator isolation semantics.

**Violation symptom.** Scan blocked all writes (old design); or scan sees torn interleaved writes (if lock too short without copy).

**Enforced by.** `scan.go` snapshot path. [scan-lock-contention](../postmortems/scan-lock-contention.md).

---

## Write path and batching

### Invariant W1 — `pendingBatch` records are not dropped during async flush

If new records are appended to `pendingBatch` while the batch flusher holds an in-flight batch (during WAL `fsync`), those new records must remain queued after flush completes.

`db.pendingBatch = batch[:0]` during concurrent append dropped queued records (384/50000 keys in stress test).

**Violation symptom.** `Get: key not found` for keys where `Put` returned nil; partial dataset after rapid puts.

**Enforced by.** `flushPendingBatch` only reuses slice when `len(db.pendingBatch) == 0`.

**Test.** `TestRapidPutNoLossDuringAsyncFlush` in `flush_test.go`

---

### Invariant W2 — `Put` return does not imply durability unless `SyncWrites` or batch sync path completed

Default: `Put` may return while records sit in `pendingBatch` or before timer-based flush. `Sync()` drains pending batch via `awaitBatchPersist`. `SyncWrites: true` forces persist per qualifying write.

**Enforced by.** `write.go`, `sync.go`, CLI `sync` command.

---

## Compaction

### Invariant C1 — Compaction merge preserves tombstones until keys are overwritten

`MergeReadersKeepTombstones` emits tombstone entries. Duplicate keys resolve to the newer file (higher index in `db.sstables`).

Deletes must survive compaction or resurrect after merge.

**Violation symptom.** Deleted keys reappear after compaction.

---

### Invariant C2 — Input SST files are not deleted until manifest commit succeeds and refs drain

Order: merge to disk → manifest `SetFileSet` + fsync → update `db.sstables` → `Discard` inputs → delete when refs=0.

Crash after manifest commit must not require input files for recovery; crash before commit must leave inputs live.

**Crash test points.** `compact_after_manifest`, `compact_after_delete_old` in `crash_recovery_test.go`

---

### Invariant C3 — Compaction failure does not block writes

Compaction errors set background `compaction` error and retry. Unlike flush, writes continue (read amplification may grow).

Scoped failure model — ingestion vs compaction independence.

---

## Recovery and open

### Invariant R1 — Single process per database directory

`LOCK` file via `flock` / `LockFileEx`. Second `Open` returns `ErrDatabaseLocked`.

Two processes appending to one WAL corrupts both.

---

### Invariant R2 — Orphan SSTs never become live on open

Files matching `^sst_\d{8}\.sst$` not in manifest are moved to `quarantine/`, not loaded.

Forensic preservation without polluting the live set.

---

### Invariant R3 — WAL replay is bounded and salvageable

Replay respects `ReplayLimits` (max key/value/file size). Partial tail record at EOF is truncated to last valid checksum.

Corrupt or attacker-controlled WAL must not OOM the process.

---

### Invariant R4 — `Open` replays WAL only after SST load and offset selection

Sequence: LOCK → manifest → load SSTs → quarantine orphans → `walReplayStartOffset` → replay → open WAL for append.

Early `Open` replayed the full WAL after SST load ([wal-replay-bug](../postmortems/wal-replay-bug.md)).

---

## Shutdown

### Invariant S1 — `Close` does not destroy WAL/manifest until workers stop or timeout

On `ErrCloseIncomplete`, abort path keeps WAL and manifest handles open so background goroutines cannot race with nil manifest. [shutdown-ordering](../postmortems/shutdown-ordering.md).

---

## Invariant summary table

| ID | Statement | Primary enforcement |
|----|-----------|-------------------|
| D1 | WAL fsync before memtable apply | `batch.go` |
| D2 | Durable keys in WAL ∪ live SSTs | D1 + D3 + D4 |
| D3 | Manifest defines live SST set | `manifest.go`, `loadSSTables` |
| D4 | Manifest fsync before memory swap | `flush.go`, `compactor.go` |
| D5 | WAL prefix redundant after checkpoint | `wal_state.go`, truncate |
| D6 | Complete SST before manifest | `sstable.Writer`, flush |
| V1 | Newest-wins read ordering | `get.go` |
| V2 | No close of referenced readers | `Ref`/`Discard` |
| V3 | Scan snapshot isolation | `scan.go` |
| W1 | No pendingBatch loss on flush | `batch.go` |
| W2 | Async Put ≠ durable | `sync.go`, docs |
| C1 | Tombstones survive merge | `MergeReadersKeepTombstones` |
| C2 | Delete inputs after manifest | `compactor.go` |
| C3 | Compaction errors scoped | `background_err` |
| R1 | One process per dir | `dir_lock_*.go` |
| R2 | Orphans quarantined | `removeOrphanSSTFiles` |
| R3 | Bounded WAL replay | `wal/limits.go` |
| R4 | SST-first open | `db.go` Open |
| S1 | Bounded shutdown | `close.go` |

---

## Verification

| Method | Invariants covered |
|--------|-------------------|
| `go test -race ./...` | V2, concurrency generally |
| `PEBBLEDB_CRASH_AT` subprocess tests | D4, D5, C2 crash windows |
| `wal_state_test.go` | D5 offset edge cases |
| `TestRapidPutNoLossDuringAsyncFlush` | W1 |
| `manifest_test.go` | D3, rotation |
| `TestGetSurvivesCompactionWithHeldRefs` | V2 |
