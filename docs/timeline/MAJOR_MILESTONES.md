# Major milestones

## M1 — Durable write loop

WAL + skip list memtable + Put/Get.

## M2 — Immutable SST layer

Block SSTables with flush from memtable.

## M3 — Authoritative manifest

Live SST set in `MANIFEST-*`, not directory glob.

## M4 — Background compaction

Bounded SST count via oldest-2 merge.

## M5 — Correct recovery

`wal.flush` + bounded WAL replay tail.

## M6 — Concurrent reads

Bloom filters, reader refs, scan snapshots.

## M7 — Durability API

Group commit, `Sync()`, `SyncWrites`, directory lock.
