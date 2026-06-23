# Benchmark results

Measured on **2026-06-23** on this machine:

| Setting | Value |
|---------|-------|
| CPU | Intel Core i9-14900HX |
| OS | Windows 11 amd64 |
| Go | 1.23.4 |
| Disk | local NVMe (laptop) |
| `GOMAXPROCS` | 32 (default); RandomRead pins 4 |
| `MemtableSize` | 128 MiB (read/scan preload) |
| `CompactionThreshold` | 100 (compaction held off) |
| Value size | 128 bytes |
| Key format | `key-%010d` (14 bytes) |

Command:

```powershell
go test ./internal/db -run=NonExistent "-bench=." -benchmem -count=1
```

## Sequential write (async group commit)

Each sub-benchmark runs one timed loop over the full dataset (`b.N=1`).

| Dataset | ops/sec | MB/sec | ns/op (per key) | B/op (total) | allocs/op (total) |
|---------|---------|--------|-----------------|--------------|-------------------|
| 100k | 37,709 | 5.11 | 26,519 | 83,454,424 | 1,045,268 |
| 500k | 36,667 | 4.97 | 27,273 | 436,613,456 | 5,454,917 |
| 1M | 33,495 | 4.54 | 29,855 | 883,065,984 | 11,024,440 |

## Random read (parallel Get, memtable-only)

50k keys preloaded; `GOMAXPROCS=4`, `-benchtime=3s`:

| Dataset | ops/sec | MB/sec | ns/op | B/op | allocs/op |
|---------|---------|--------|-------|------|-----------|
| 50k | ~3,083,000 | 438 | 324 | 129 | 1 |

## Scan throughput (memtable-only)

Each iteration scans the listed key range once. `ops/sec` is keys/sec.

| Range | ops/sec (keys) | MB/sec | ns/op (per scan) | B/op | allocs/op |
|-------|----------------|--------|------------------|------|-----------|
| 1k | 199,455 | 27.0 | 5,013,656 | 10,002,070 | 100,010 |
| 10k | 1,915,905 | 259.5 | 5,219,465 | 10,002,068 | 100,010 |
| 50k | 7,638,287 | 1,034 | 6,545,970 | 10,002,056 | 100,009 |

## Notes

- Write numbers include memtable insert + batched WAL append; no per-key fsync unless batch thresholds fire.
- Read/scan benches preload into a large memtable and never close/reopen — they measure hot memtable paths, not full LSM depth.
- On a prior run, group commit was ~20× faster than `SyncWrites: true` for the same payload (`01eef8e`). Re-measure on your disk before comparing to RocksDB or SQLite.

Reproduce:

```powershell
go test ./internal/db -run=NonExistent "-bench=." -benchmem -count=1
go test ./internal/db -run=NonExistent -bench=BenchmarkRandomRead -benchmem -count=1 -benchtime=3s
```
