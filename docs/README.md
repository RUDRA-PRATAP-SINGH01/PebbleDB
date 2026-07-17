# PebbleDB documentation

Engineering notes for the LSM implementation in this repo.

## Repository layout

```
PebbleDB/
├── cmd/pebbledb/          CLI (put, get, delete, scan, sync)
├── internal/
│   ├── db/                Database API, workers, recovery
│   ├── wal/               Write-ahead log
│   ├── memtable/          Skip-list memtable
│   ├── sstable/           Immutable on-disk tables
│   ├── manifest/          Live SST set log
│   ├── iterator/          K-way merge iterator
│   └── bloom/             Per-SST bloom filters
├── docs/                  Architecture, design, testing, diagrams
├── .github/workflows/     CI (ubuntu + macos)
├── go.mod
├── README.md
├── CONTRIBUTING.md
└── LICENSE
```

Library import path: `github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db`

## Start here

| Topic | File |
|-------|------|
| Architecture | [architecture/SYSTEM_OVERVIEW.md](architecture/SYSTEM_OVERVIEW.md) |
| Invariants | [design/INVARIANTS.md](design/INVARIANTS.md) |
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

- [INVARIANTS.md](design/INVARIANTS.md)
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
- [ATF.md](testing/ATF.md) — acceptance crash-recovery framework
- [CRASH_TESTING.md](testing/CRASH_TESTING.md)
- [TestRecoveryAfterFlushCrash_ATDD_Spec.md](TestRecoveryAfterFlushCrash_ATDD_Spec.md)
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

- [diagrams/README.md](diagrams/README.md) — Mermaid source index
