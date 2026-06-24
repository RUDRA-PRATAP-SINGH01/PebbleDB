# Engineering decisions

Decisions with alternatives rejected.

## LSM over B-tree

I chose an LSM because append-only WAL + immutable SSTs map directly to how I wanted to learn crash recovery. B-trees would have forced page-oriented write amplification and obscured the durability story.

Rejected: BoltDB-style single-file B+tree (harder to teach flush/compaction boundaries).

## Skip list memtable

Concurrent `Put` with sorted order. Easier for me to implement than a concurrent B-tree.

Rejected: sorted slice (O(n) inserts), red-black tree (rotation complexity under concurrency).

## Single `db.mu` writer lock

One big mutex for write path and structure changes. Correctness over throughput.

Rejected: per-memtable locks without a clear plan for flush handoff.

## Group commit default

Async WAL batching with 1ms delay. Measured ~20× write throughput vs per-op fsync (commit `01eef8e`).

Rejected: synchronous-only API — too slow for bulk load benchmarks.

Explicit escape hatches: `Sync()`, `SyncWrites`.

## Manifest as live set authority

Glob of `sst_*.sst` breaks after compaction crashes leave orphans.

Rejected: directory listing as source of truth.

## Manifest-before-memory on compaction

Swap `sstables` only after manifest fsync.

Rejected: memory-first swap (caused post-crash divergence — see postmortem).

## wal.flush checkpoint

16-byte sidecar to bound WAL replay after flush.

Rejected: replay entire WAL forever; rejected WAL sequence numbers in records (heavier format change).

## Tombstones everywhere

Deletes are records, not physical removal.

Rejected: immediate purge (breaks snapshot semantics and crash recovery).

## Oldest-2 compaction

Simplest merge policy to implement and test.

Rejected: leveled compaction (too many moving parts before recovery was solid).

## Bloom per SST

Skip whole files on negative lookup.

Rejected: no filter (read amp too high), global filter (invalidation hard).

## Scan via memtable snapshot

Copy-on-read under brief lock.

Rejected: long-held `RLock` (blocked all writes).

## Background error scope

WAL/flush block writes; compaction does not. Reads never blocked.

Rejected: global read-only mode on any background error.

## Directory LOCK file

`flock` / `LockFileEx` for single-process open.

Rejected: no lock (silent corruption risk with two processes).

## Quarantine orphans

Move unknown SSTs to `quarantine/` instead of delete.

Rejected: `os.Remove` on open (destroys forensic evidence).

## CI without Windows

Windows `os.Rename` with open handles caused flaky CI. I test ubuntu + macos only in `.github/workflows/ci.yml`.

Rejected: fighting CI on windows-latest for a project I deploy on Linux.
