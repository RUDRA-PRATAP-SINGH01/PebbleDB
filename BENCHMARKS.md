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

## Durability semantics

PebbleDB separates **acknowledgement** (API return) from **durability** (survives crash/power loss). Understanding this is required to interpret write benchmarks and to use the CLI safely.

### Default: group commit (async)

Unless configured otherwise, `Put` and `Delete`:

1. Append the record to an in-memory `pendingBatch`
2. Return `nil` immediately (often within ~1 ms, or sooner when batch thresholds are hit)
3. A background `batchFlusher` goroutine later calls `wal.AppendBatch` + **one fsync** for the whole batch

| Threshold | Value | Effect |
| :--- | :--- | :--- |
| `batchFlushDelay` | 1 ms | Timer-based flush of pending batch |
| `batchMaxRecords` | 64 | Force synchronous persist when batch is full |
| `batchMaxBytes` | 16 KiB | Force synchronous persist when batch is large |
| Memtable pressure | `active.Size() + batch > MemtableSize` | Force synchronous persist |

When any threshold triggers, the caller blocks in `awaitBatchPersist()` until WAL fsync completes.

**Crash window:** If the process dies after `Put` returns `nil` but before the batch flusher fsyncs, the write is lost even though the API reported success. In-process `Get` may still see the key via `lookupPendingBatch` while the record is only in RAM.

### Explicit durability barriers

| Mechanism | When to use |
| :--- | :--- |
| `DB.Sync()` | After a batch of async puts; waits until all queued records are WAL-fsynced |
| `Options.SyncWrites = true` | Every `Put`/`Delete` blocks until its WAL fsync completes (lowest throughput, strongest per-op guarantee) |
| `Close()` | Full shutdown: drains batches, flushes memtables, syncs WAL |

### CLI

```bash
# Async (default) — fast, call sync when needed
./pebbledb put key value
./pebbledb sync

# Synchronous mode — each put waits for fsync
./pebbledb -sync-writes put key value

# Or via environment
export PEBBLEDB_SYNC_WRITES=1
./pebbledb put key value
```

### What benchmarks measure

`BenchmarkSequentialWrite` uses the **default async group-commit path**. Reported `ops/sec` includes time spent in the benchmark loop but **does not** include fsync latency for every key unless a threshold forces `awaitBatchPersist`. Throughput numbers therefore reflect amortized WAL durability, not per-write synchronous disk latency.

To benchmark synchronous durability:

```bash
go test -bench=BenchmarkSequentialWrite -benchmem -count=1 ./internal/db
# Compare against a local patch or wrapper that sets Options{SyncWrites: true}
```

Or measure end-to-end latency with `Sync()` after each put in a custom micro-benchmark.

### Durability vs flush/compaction

| Stage | Survives crash after... |
| :--- | :--- |
| `Put` returns (async) | **Not guaranteed** — only RAM / pending batch |
| `Sync()` or batch fsync | WAL replay on reopen |
| Memtable flush + manifest commit | SST on disk; WAL tail may still exist until truncate |
| `Close()` success | Full durable shutdown |

Reads (`Get`, `Scan`) continue from memtable + SST even when WAL or flush background errors block new writes.

---

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

- Write throughput vs memtable size (4 MiB default): ingest is fast while the active memtable absorbs writes; periodic flushes to SST + **batched** WAL fsync produce visible stalls.
- **Async vs sync writes:** default group commit inflates write `ops/sec` versus `-sync-writes` / `Sync()` per batch; report which mode was used.
- Random read latency with bloom filters: most lookups skip block IO when the key is absent from older SSTs; fully flushed 500k-key dataset exercises the full LSM read path.
- Scan scaling: 1k vs 10k vs 100k keys — linear growth in keys/sec indicates merge-iterator overhead dominates at small ranges; flat MB/s at large ranges suggests block IO bound.

## Workload assumptions

| Benchmark | What it measures | What it excludes |
| :--- | :--- | :--- |
| `BenchmarkSequentialWrite` | Memtable insert + group-commit WAL batching + background flush | Per-key synchronous fsync (unless batch threshold hit); compaction (`CompactionThreshold=100`) |
| `BenchmarkRandomRead` | Concurrent `Get` against flushed data | Write contention (read-only phase) |
| `BenchmarkScanThroughput` | Merge-iterator range scan over SST layers | Live writes during scan (snapshot at `Scan()` time) |

When comparing against RocksDB/Pebble or other engines, note whether those numbers use `sync=true` per write. PebbleDB defaults to async group commit; use `SyncWrites` or `Sync()` for apples-to-apples durable-write benchmarks.

## Reproducibility notes

- Benchmarks use `b.TempDir()` — no hardcoded paths, automatic cleanup.
- Random read uses a per-goroutine `rand.Rand` to avoid lock contention on the global source.
- Preload closes and reopens the database so reads/scans hit persisted SSTables, not only the active memtable.
- Results vary with disk (NVMe vs SATA), CPU governor, and background flush timing. Run at least three times and take the median.
