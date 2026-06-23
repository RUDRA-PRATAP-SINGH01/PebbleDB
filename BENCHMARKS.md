# PebbleDB Benchmarks

This document records throughput and latency numbers for PebbleDB under fintech-style workloads: ordered ingest, concurrent point lookups, and range scans over flushed SSTables.

Run the suite locally (no race detector — it skews timing):

```bash
go test -bench=. -benchmem -count=1 ./internal/db
```

Targeted runs:

```bash
go test -bench=BenchmarkSequentialWrite -benchmem -count=1 ./internal/db
go test -bench=BenchmarkRandomRead -benchmem -count=1 ./internal/db
go test -bench=BenchmarkScanThroughput -benchmem -count=1 ./internal/db
```

`-count=1` disables the Go test cache so filesystem benchmarks are not served from a stale cached pass.

## Environment

| Setting | Value |
| :--- | :--- |
| Machine | MacBook Pro M2 Pro / Ubuntu 22.04 LTS (NVMe SSD) |
| Go version | 1.23.4 |
| Memtable size | 4 MiB (default) |
| Compaction threshold (benchmarks) | 100 (compaction held off during write bench) |
| Value payload | 128 bytes (transaction-metadata-shaped records) |
| Key format | `key-%010d` (lexicographic, 14 bytes) |

## Benchmark Results

| Dataset Size | Operation | Throughput (ops/sec) | Latency (µs/op) | Memory Alloc (B/op) | Disk Used (MB) |
| :--- | :--- | :--- | :--- | :--- | :--- |
| 100,000 | Sequential Write | XX,XXX | XX µs | XX,XXX | XX MB |
| 500,000 | Sequential Write | XX,XXX | XX µs | XX,XXX | XX MB |
| 1,000,000 | Sequential Write | XX,XXX | XX µs | XX,XXX | XX MB |
| 500,000 | Random Read (Parallel, GOMAXPROCS=4) | XX,XXX | XX µs | XX,XXX | - |
| 100,000 | Scan (1k keys) | XX,XXX | - | XX,XXX | - |
| 100,000 | Scan (10k keys) | XX,XXX | - | XX,XXX | - |
| 100,000 | Scan (100k keys) | XX,XXX | - | XX,XXX | - |

Custom metrics (`ops/sec`, `MB/sec`) appear in the benchmark line when using `-benchmem`.

### How to read `go test` output

Example line (values are placeholders):

```
BenchmarkSequentialWrite/100k-8    1    4523890123 ns/op    142 B/op    2 allocs/op    22103 ops/sec    3.01 MB/sec
```

- `ns/op` — wall time for the full sub-benchmark (not per-key); divide by dataset size for per-key latency.
- `B/op` / `allocs/op` — allocations attributed to the benchmark loop.
- `ops/sec` / `MB/sec` — custom metrics reported via `b.ReportMetric`.

Disk used: measure the benchmark temp directory size after a run, or inspect the data dir left by a manual `Open` with the same dataset count.

## Interpretation

[Placeholder — paste your analysis after running locally.]

Suggested angles to cover:

- Write throughput vs memtable size (4 MiB default): ingest is fast while the active memtable absorbs writes; periodic flushes to SST + WAL sync produce visible stalls.
- Random read latency with bloom filters: most lookups skip block IO when the key is absent from older SSTs; fully flushed 500k-key dataset exercises the full LSM read path.
- Scan scaling: 1k vs 10k vs 100k keys — linear growth in keys/sec indicates merge-iterator overhead dominates at small ranges; flat MB/s at large ranges suggests block IO bound.

## Workload assumptions

| Benchmark | What it measures | What it excludes |
| :--- | :--- | :--- |
| `BenchmarkSequentialWrite` | WAL fsync + memtable insert + background flush | Compaction (`CompactionThreshold=100`) |
| `BenchmarkRandomRead` | Concurrent `Get` against flushed data | Write contention (read-only phase) |
| `BenchmarkScanThroughput` | Merge-iterator range scan over SST layers | Live writes during scan (snapshot at `Scan()` time) |

## Reproducibility notes

- Benchmarks use `b.TempDir()` — no hardcoded paths, automatic cleanup.
- Random read uses a per-goroutine `rand.Rand` to avoid lock contention on the global source.
- Preload closes and reopens the database so reads/scans hit persisted SSTables, not only the active memtable.
- Results vary with disk (NVMe vs SATA), CPU governor, and background flush timing. Run at least three times and take the median.
