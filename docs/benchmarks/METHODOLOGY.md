# Benchmark methodology

How I run benchmarks and how to interpret numbers without lying.

## Environment

Record these when publishing results:

| Setting | Typical value |
|---------|----------------|
| Go version | 1.23.4 (`go.mod`) |
| OS / disk | NVMe vs SATA matters |
| `GOMAXPROCS` | default vs fixed |
| `MemtableSize` | 4 MiB default |
| `CompactionThreshold` | 100 in benches (compaction held off) |
| Value size | 128 bytes (`benchPayload`) |
| Key format | `key-%010d` (14 bytes) |

## Commands

```bash
# full suite
go test -bench=. -benchmem -count=1 ./internal/db

# targeted
go test -bench=BenchmarkSequentialWrite -benchmem -count=1 -benchtime=3s ./internal/db
go test -bench=BenchmarkRandomRead -benchmem -count=1 -benchtime=3s ./internal/db
go test -bench=BenchmarkScanThroughput -benchmem -count=1 ./internal/db
```

`-count=1` disables Go test cache — important for filesystem benchmarks.

On PowerShell, quote flags with dots:

```powershell
go test "-bench=." -benchmem -count=1 ./internal/db
```

## Async vs sync writes

| Mode | How to benchmark |
|------|------------------|
| Default group commit | `BenchmarkSequentialWrite` as-is |
| Per-op durable | `Options{SyncWrites: true}` in custom bench or CLI `-sync-writes` |

Comparing to RocksDB `sync=true` without matching mode is invalid.

## What each benchmark includes / excludes

| Benchmark | Includes | Excludes |
|-----------|----------|----------|
| SequentialWrite | memtable insert, batch WAL, background flush | per-key sync unless threshold hit |
| RandomRead | bloom + block IO on memtable-only dataset in current bench | SST cold path (preload stays in memtable unless changed) |
| ScanThroughput | merge iterator over memtable | concurrent writes during scan |

`BenchmarkRandomRead` preloads into a large memtable and does not close/reopen. It measures hot memtable reads, not full LSM depth.

## Custom metrics

`reportThroughput` emits `ops/sec` and `MB/sec` via `b.ReportMetric`.

## Reproducibility

- `b.TempDir()` for isolation
- Per-goroutine `rand.Rand` in parallel read bench
- Run ≥3 times, report median
- Watch for flush stalls during write bench — variance is expected
