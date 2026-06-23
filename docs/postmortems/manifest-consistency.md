# Context

Compaction and manifest rotation are the highest-risk durability paths in PebbleDB. I lost trust in my own engine twice: once when in-memory `sstables` diverged from manifest after a crash, and again when manifest rotation left `CURRENT` pointing at a partial file.

# Original Design

Early compaction:

1. Merge two SSTables in memory / temp file.
2. Swap `db.sstables` slice to drop inputs and add output.
3. Append manifest records (sometimes later).
4. Delete old files.

Manifest rotation initially rewrote the log in place or updated `CURRENT` before the new manifest was fully durable.

# Why I Thought It Was Correct

The merged SST was complete on disk before I touched memory. I treated the manifest as bookkeeping that could catch up.

# Failure Symptoms

- After crash + reopen: manifest listed SST set A, disk had SST set B, `Get` returned `ErrNotFound` for keys I had written.
- Orphan SST files appeared after compaction crash — live data in files not referenced by manifest.
- Manifest replay sometimes failed mid-record on Windows because the file was still open during truncate.

# Investigation

I added crash-point tests (`PEBBLEDB_CRASH_AT=compact_after_manifest`) and inspected directory state after forced exit. The pattern was clear: memory had already dropped input SST ids before manifest fsync, or `CURRENT` rotated before the new manifest file was complete.

Commits:
- `054e6f7` / `4be43d4` — manifest ordering, bloom panic, WAL truncate guards
- `0b2baf0` — SST lifecycle and compaction atomicity
- `fd701a3` — atomic manifest rotation (C1, C2, C3 data loss fixes)

# Root Cause

1. **Manifest after memory** — crash window where process state and durable metadata disagree.
2. **Non-atomic CURRENT update** — readers could open a truncated manifest.
3. **Glob-based SST discovery** — orphan files confused recovery before I added manifest-only live set.

# Fix

Compaction now:

1. Merge to new SST on disk.
2. `manifest.AppendSetFileSet` + **fsync** (durable boundary).
3. Swap in-memory `sstables` only after manifest succeeds.
4. On failure: delete merged file, leave memory untouched.
5. If readers invalidated between pick and commit: roll back manifest to `oldLiveIDs`.

Manifest rotation (`MaybeCompact`):

- Write new `MANIFEST-NNNNNN` with a single `SetFileSet` snapshot.
- Fsync, close handles, then atomic `CURRENT` rename via temp file.

Flush uses `AppendNewFile` with the same manifest-first rule.

# Verification

- `crash_recovery_test.go` — compaction crash points.
- `manifest_test.go` — replay, rotation, concurrent append tests.
- `TestManifestIgnoresOrphanSSTAfterCompactionCrash` — orphan SST must not affect reads.

# Lessons Learned

- I treat manifest fsync as the moment a file becomes **live**. Everything before that is provisional.
- Rolling back manifest on reader invalidation is cheaper than debugging silent data loss later.
- `quarantine/` for orphan SSTs is safer than `os.Remove` during recovery — I can inspect mistakes.
