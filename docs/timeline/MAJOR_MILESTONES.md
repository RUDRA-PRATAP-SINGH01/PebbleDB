# Major milestones

Milestones that changed what PebbleDB **is**, not every refactor.

## M1 — Durable write loop

WAL + skip list memtable + Put/Get.

**Proof:** WAL replay tests, memtable unit tests.

## M2 — Immutable SST layer

Block SSTables with flush from memtable.

**Proof:** `TestFlushToSSTable`, `TestReopenLoadsSSTables`.

## M3 — Authoritative manifest

Live SST set in `MANIFEST-*`, not directory glob.

**Proof:** `TestFlushWritesManifestRecord`, orphan quarantine tests.

## M4 — Background compaction

Bounded SST count via oldest-2 merge.

**Proof:** `TestCompactionMergesDuplicateKeys`, crash tests at compact boundaries.

## M5 — Correct recovery

`wal.flush` + bounded WAL replay tail.

**Proof:** `wal_state_test.go`, crash recovery suite.

## M6 — Concurrent reads

Bloom filters, reader refs, scan snapshots.

**Proof:** `-race` CI, `TestScanDoesNotBlockWrites`.

## M7 — Production-shaped durability API

Group commit, `Sync()`, `SyncWrites`, directory lock.

**Proof:** `durability_test.go`, CLI sync command.

## M8 — Documented engineering record

`docs/` structure with postmortems and architecture split from README.

**Proof:** you are reading it.

## What is not a milestone

- Lint/format fixes
- Benchmark wrapper cleanup
- gitignore path tweaks

Those matter for hygiene, not for storage semantics.

## Related

- [DEVELOPMENT_TIMELINE.md](DEVELOPMENT_TIMELINE.md)
- [../README.md](../README.md)
