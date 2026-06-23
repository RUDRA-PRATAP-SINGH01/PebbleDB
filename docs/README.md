# PebbleDB documentation

I wrote PebbleDB from scratch to learn how a real log-structured merge tree behaves under crashes, concurrency, and compaction pressure. This folder is the engineering record — not a tutorial on what an LSM is.

## How to read this

| If you want… | Start here |
|--------------|------------|
| A 5-minute mental model | [architecture/SYSTEM_OVERVIEW.md](architecture/SYSTEM_OVERVIEW.md) |
| Why I made specific choices | [design/DECISIONS.md](design/DECISIONS.md) |
| What broke and how I fixed it | [postmortems/](postmortems/) |
| How the engine evolved over time | [design/EVOLUTION.md](design/EVOLUTION.md) |
| How I test durability | [testing/TESTING_STRATEGY.md](testing/TESTING_STRATEGY.md) |
| Benchmark methodology | [benchmarks/METHODOLOGY.md](benchmarks/METHODOLOGY.md) |

## Architecture

- [SYSTEM_OVERVIEW.md](architecture/SYSTEM_OVERVIEW.md) — layers, on-disk layout, package map
- [WRITE_PATH.md](architecture/WRITE_PATH.md) — Put/Delete, group commit, flush triggers
- [READ_PATH.md](architecture/READ_PATH.md) — Get layering, bloom, reader refs
- [RECOVERY.md](architecture/RECOVERY.md) — Open sequence, wal.flush, orphan handling
- [CONCURRENCY_MODEL.md](architecture/CONCURRENCY_MODEL.md) — locks, workers, scan snapshots
- [SSTABLE_DESIGN.md](architecture/SSTABLE_DESIGN.md) — file format, blocks, footer
- [MANIFEST_DESIGN.md](architecture/MANIFEST_DESIGN.md) — live set, rotation, atomic CURRENT
- [COMPACTION.md](architecture/COMPACTION.md) — oldest-2 merge, manifest ordering
- [WAL_DESIGN.md](architecture/WAL_DESIGN.md) — record format, truncate, replay limits

## Design record

- [DECISIONS.md](design/DECISIONS.md) — decisions with rejected alternatives
- [TRADEOFFS.md](design/TRADEOFFS.md) — accepted costs
- [EVOLUTION.md](design/EVOLUTION.md) — phased engineering narrative
- [LESSONS_LEARNED.md](design/LESSONS_LEARNED.md) — patterns I would reuse or avoid

## Postmortems (real bugs only)

- [wal-replay-bug.md](postmortems/wal-replay-bug.md) — replaying flushed WAL bytes
- [manifest-consistency.md](postmortems/manifest-consistency.md) — manifest/memory ordering and rotation
- [compaction-race.md](postmortems/compaction-race.md) — reader lifetime vs compaction
- [reader-lifecycle.md](postmortems/reader-lifecycle.md) — Ref/Unref and Windows file locks
- [scan-lock-contention.md](postmortems/scan-lock-contention.md) — scan blocking writes
- [shutdown-ordering.md](postmortems/shutdown-ordering.md) — Close timeouts and incomplete shutdown

## Testing

- [TESTING_STRATEGY.md](testing/TESTING_STRATEGY.md)
- [CRASH_TESTING.md](testing/CRASH_TESTING.md)
- [RACE_DETECTION.md](testing/RACE_DETECTION.md)
- [FAILURE_INJECTION.md](testing/FAILURE_INJECTION.md)

## Benchmarks

- [BENCHMARKS.md](benchmarks/BENCHMARKS.md) — suite overview
- [METHODOLOGY.md](benchmarks/METHODOLOGY.md) — how to run and interpret
- [RESULTS.md](benchmarks/RESULTS.md) — numbers (fill locally)

## Timeline

- [DEVELOPMENT_TIMELINE.md](timeline/DEVELOPMENT_TIMELINE.md)
- [MAJOR_MILESTONES.md](timeline/MAJOR_MILESTONES.md)

## Diagrams

Source Mermaid files live in [diagrams/](diagrams/). Architecture pages embed the relevant diagrams inline.

## Status

PebbleDB is a single-node embedded engine I use to study storage internals. I do not claim production readiness. See the root [README](../README.md) for scope and limits.
