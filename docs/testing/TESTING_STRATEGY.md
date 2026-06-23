# Testing strategy

I test PebbleDB in layers because storage bugs hide at integration boundaries.

## Pyramid

```
                    crash subprocess tests
                 integration (internal/db)
              package tests (wal, manifest, sstable, ...)
           unit tests (bloom, iterator, memtable)
```

| Layer | Package | What it proves |
|-------|---------|----------------|
| Unit | `memtable`, `bloom`, `iterator` | Data structure invariants |
| Storage | `wal`, `manifest`, `sstable` | On-disk format + salvage |
| Integration | `db` | Open/flush/compaction/recovery |
| Crash | `db` subprocess | Durability at specific boundaries |
| Race | all with `-race` | Concurrent access bugs |

## What I always run locally

```bash
go test ./...
go test ./... -race
go test ./internal/db -run Crash -v
```

## What CI runs

From `.github/workflows/ci.yml`:

- `go vet ./...`
- `golangci-lint`
- `go test -race -count=1 -shuffle=on -coverprofile=coverage.out ./...`

I disable test cache in CI (`-count=1`) because WAL/manifest truncation bugs hid behind cached passes.

I excluded `windows-latest` from CI — file lock races on rename with open handles caused false reds.

## Coverage philosophy

High coverage in `internal/db` (~79%) matters more than 100% in `bloom`. I prioritize paths that touch durability ordering.

## Test naming

Integration tests name the invariant:

- `TestWalReplayStartOffsetWhenWalTruncatedBelowFreeze`
- `TestManifestIgnoresOrphanSSTAfterCompactionCrash`
- `TestGetSurvivesCompactionWithHeldRefs`

If the name is vague, the test is probably weak.

## Related

- [CRASH_TESTING.md](CRASH_TESTING.md)
- [RACE_DETECTION.md](RACE_DETECTION.md)
- [FAILURE_INJECTION.md](FAILURE_INJECTION.md)
