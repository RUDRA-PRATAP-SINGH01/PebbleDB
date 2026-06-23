# PebbleDB documentation

Engineering notes for the LSM implementation in this repo.

## Start here

| Topic | File |
|-------|------|
| Architecture | [architecture/SYSTEM_OVERVIEW.md](architecture/SYSTEM_OVERVIEW.md) |
| Decisions | [design/DECISIONS.md](design/DECISIONS.md) |
| Bugs and fixes | [postmortems/](postmortems/) |
| Evolution | [design/EVOLUTION.md](design/EVOLUTION.md) |
| Testing | [testing/TESTING_STRATEGY.md](testing/TESTING_STRATEGY.md) |
| Benchmarks | [benchmarks/METHODOLOGY.md](benchmarks/METHODOLOGY.md) |

## Architecture

- [SYSTEM_OVERVIEW.md](architecture/SYSTEM_OVERVIEW.md)
- [WRITE_PATH.md](architecture/WRITE_PATH.md)
- [READ_PATH.md](architecture/READ_PATH.md)
- [RECOVERY.md](architecture/RECOVERY.md)
- [CONCURRENCY_MODEL.md](architecture/CONCURRENCY_MODEL.md)
- [SSTABLE_DESIGN.md](architecture/SSTABLE_DESIGN.md)
- [MANIFEST_DESIGN.md](architecture/MANIFEST_DESIGN.md)
- [COMPACTION.md](architecture/COMPACTION.md)
- [WAL_DESIGN.md](architecture/WAL_DESIGN.md)

## Design

- [DECISIONS.md](design/DECISIONS.md)
- [TRADEOFFS.md](design/TRADEOFFS.md)
- [EVOLUTION.md](design/EVOLUTION.md)
- [LESSONS_LEARNED.md](design/LESSONS_LEARNED.md)

## Postmortems

- [wal-replay-bug.md](postmortems/wal-replay-bug.md)
- [manifest-consistency.md](postmortems/manifest-consistency.md)
- [compaction-race.md](postmortems/compaction-race.md)
- [reader-lifecycle.md](postmortems/reader-lifecycle.md)
- [scan-lock-contention.md](postmortems/scan-lock-contention.md)
- [shutdown-ordering.md](postmortems/shutdown-ordering.md)

## Testing

- [TESTING_STRATEGY.md](testing/TESTING_STRATEGY.md)
- [CRASH_TESTING.md](testing/CRASH_TESTING.md)
- [RACE_DETECTION.md](testing/RACE_DETECTION.md)
- [FAILURE_INJECTION.md](testing/FAILURE_INJECTION.md)

## Benchmarks

- [BENCHMARKS.md](benchmarks/BENCHMARKS.md)
- [METHODOLOGY.md](benchmarks/METHODOLOGY.md)
- [RESULTS.md](benchmarks/RESULTS.md)

## Timeline

- [DEVELOPMENT_TIMELINE.md](timeline/DEVELOPMENT_TIMELINE.md)
- [MAJOR_MILESTONES.md](timeline/MAJOR_MILESTONES.md)

## Diagrams

Mermaid sources: [diagrams/](diagrams/)
