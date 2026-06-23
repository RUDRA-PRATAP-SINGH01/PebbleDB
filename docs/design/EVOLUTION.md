# Evolution

This is the engineering story of PebbleDB — not a commit log. Each phase is something I built, broke, and fixed.

## Phase 1: WAL + memtable

**Motivation.** Prove append-only durability and sorted in-memory store.

**What I built.** `internal/wal` CRC records, `internal/memtable` skip list, `Put`/`Get` against active memtable only.

**Problems.** No persistence beyond RAM if WAL replay was wrong. Windows WAL truncate failed with open handles.

**Fixes.** Cross-platform truncate via close-truncate-reopen (`78e8eb8`). Unified `writeRecord` path for put/delete.

**Lesson.** Platform file semantics are part of the storage engine.

---

## Phase 2: SSTables

**Motivation.** Memtable must become immutable on-disk runs.

**What I built.** Block-based SST writer/reader, index, footer v2, flush from memtable iterator.

**Problems.** Tombstones invisible to early readers. Partial files visible if renamed too early.

**Fixes.** Tombstone byte in block entries. Write to `.tmp`, rename on `Close`, manifest learns file only after rename.

**Lesson.** Immutability starts at rename + manifest, not at last byte written.

---

## Phase 3: Manifest

**Motivation.** Glob-based SST discovery broke after crashes left orphan files.

**What I built.** Append-only `MANIFEST-*`, `CURRENT` pointer, `NewFile` / `SetFileSet` records.

**Problems.** Memory/manifest ordering bugs. Rotation truncated open files on Windows.

**Fixes.** Manifest-before-memory rule. Atomic `CURRENT` update (`fd701a3`).

**Lesson.** The manifest is law — disk files are candidates until listed.

---

## Phase 4: Compaction

**Motivation.** SST count grew without bound.

**What I built.** Background compactor, oldest-2 merge, tombstones preserved.

**Problems.** Race with `Get`. Manifest/memory divergence on crash.

**Fixes.** Reader `Ref`/`Unref`, `readersStillPresent`, manifest rollback (`0b2baf0`, `cfbbf5a`).

**Lesson.** Compaction is concurrent with reads even if SSTs are "immutable."

---

## Phase 5: Bloom filters

**Motivation.** `Get` latency linear in SST count.

**What I built.** Per-file bloom in footer, `MayContain` gate before block IO.

**Problems.** Corrupt bloom metadata caused divide-by-zero panic.

**Fixes.** Reject `m==0`/`k==0` on decode (`054e6f7`).

**Lesson.** Defensive decode on untrusted disk bytes.

---

## Phase 6: Recovery redesign

**Motivation.** Full WAL replay after flush duplicated state.

**What I built.** `wal.flush` checkpoint, `walReplayStartOffset`, SST-first open.

**Problems.** Truncated WAL below freeze offset, unknown SST id in checkpoint.

**Fixes.** Replay from 0 when `wal.size < FreezeOffset`. Ignore checkpoint if SST not in manifest.

**Lesson.** Recovery is a byte-range problem, not a boolean "replay WAL yes/no."

---

## Phase 7: Concurrency fixes

**Motivation.** Scan blocked writes; compaction raced with reads; flush queue stuck.

**What I built.** Memtable snapshots, `pendingFlush` queue with drain-all flusher, reader lifecycle.

**Problems.** Long-held iterator locks. Coalesced flush signals dropping work.

**Fixes.** Snapshot copy (`scan-lock-contention` postmortem). Drain entire queue per wakeup.

**Lesson.** Liveness bugs show up in benchmarks before correctness tests fail.

---

## Phase 8: Performance and durability API

**Motivation.** Write throughput and explicit durability contracts.

**What I built.** Group commit (`01eef8e`), async batch flusher, LRU block cache (`052812d`), `Sync()` / `SyncWrites` (`0a7a5fa`).

**Problems.** Callers thought `Put` return meant durable. CI bench wrappers lied about metrics (`4ceee30`).

**Fixes.** Documented async semantics. CLI `sync` command. Removed broken bench helpers.

**Lesson.** Performance work without API clarity creates operational bugs.

---

## Phase 9: Hardening for real shutdown

**Motivation.** `Close` hung or tore down resources while workers ran.

**What I built.** Bounded drain timeouts, `ErrCloseIncomplete`, abort path keeps WAL/manifest open (`505578a`).

**Problems.** Stuck flush infinite loop on close. Directory lock errno mapping on Linux (`95541a8`).

**Fixes.** Worker join timeouts. Unix `EWOULDBLOCK` → `ErrDatabaseLocked`.

**Lesson.** Shutdown paths need the same attention as write paths.

---

## Where I am now

PebbleDB is a complete embedded LSM for learning and experimentation. The remaining gaps I care about are leveled compaction, MVCC, and fuzz testing — not more README length.

See [MAJOR_MILESTONES.md](../timeline/MAJOR_MILESTONES.md) for dates.
