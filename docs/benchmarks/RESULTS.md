# Benchmark results

I do not check in fixed numbers — they depend on your machine and disk. Fill this page after running locally.

## How to generate

```bash
go test -bench=. -benchmem -count=3 ./internal/db | tee bench.txt
```

Paste medians into the table below.

## Results template

| Dataset | Operation | ops/sec | ns/op (total) | B/op | allocs/op | Notes |
|---------|-----------|---------|---------------|------|-----------|-------|
| 100k | SequentialWrite | | | | | async group commit |
| 500k | SequentialWrite | | | | | |
| 1M | SequentialWrite | | | | | |
| 50k | RandomRead parallel | | | | | memtable-only preload |
| 50k | Scan 1k keys | | | | | |
| 50k | Scan 10k keys | | | | | |
| 50k | Scan 50k keys | | | | | |

## Interpretation checklist

When I analyze results I ask:

1. Was `SyncWrites` on or off?
2. Did flushes trigger during write bench (watch SST file creation)?
3. Is read bench hitting memtable only or SST layers?
4. Thermal throttling / background OS IO?

## Honest baseline

On my development machine, group commit improved write throughput by roughly an order of magnitude vs per-write fsync (commit `01eef8e` message: ~20×). Exact numbers vary — measure yours.

## Related

- [METHODOLOGY.md](METHODOLOGY.md)
