# Benchmark results

Numbers depend on machine and disk. Run locally and paste medians here.

```bash
go test -bench=. -benchmem -count=3 ./internal/db | tee bench.txt
```

| Dataset | Operation | ops/sec | ns/op (total) | B/op | allocs/op | Notes |
|---------|-----------|---------|---------------|------|-----------|-------|
| 100k | SequentialWrite | | | | | async group commit |
| 500k | SequentialWrite | | | | | |
| 1M | SequentialWrite | | | | | |
| 50k | RandomRead parallel | | | | | memtable-only preload |
| 50k | Scan 1k keys | | | | | |
| 50k | Scan 10k keys | | | | | |
| 50k | Scan 50k keys | | | | | |

Before comparing to other engines: check `SyncWrites` on/off, whether flushes ran during the write bench, and whether the read bench hit memtable or SST layers.

Group commit was ~20× faster than per-write fsync on my machine (`01eef8e`). Measure yours.
