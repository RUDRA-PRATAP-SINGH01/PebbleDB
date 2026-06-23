# Benchmarks overview

Benchmarks live in `internal/db/bench_test.go`. I added them to measure ingest, point lookup, and scan — not to win leaderboard comparisons.

## Suites

| Benchmark | Measures |
|-----------|----------|
| `BenchmarkSequentialWrite` | Put throughput at 100k / 500k / 1M keys |
| `BenchmarkRandomRead` | Parallel Get (GOMAXPROCS=4) over 50k keys |
| `BenchmarkScanThroughput` | Range scans 1k / 10k / 50k keys |

## Quick run

```bash
go test -bench=. -benchmem -count=1 ./internal/db
```

Do **not** use `-race` for timing — it skews results.

## Durability warning

Default write benchmarks use **async group commit**. Reported `ops/sec` does not include per-key fsync unless batch thresholds force `awaitBatchPersist`.

See [METHODOLOGY.md](METHODOLOGY.md) and [RESULTS.md](RESULTS.md).

## Moved from root

Detailed durability semantics previously in `BENCHMARKS.md` at repo root are consolidated here. Root `BENCHMARKS.md` remains as a pointer or can be removed — see [METHODOLOGY.md](METHODOLOGY.md).

## Related

- [../architecture/WRITE_PATH.md](../architecture/WRITE_PATH.md)
- [../design/TRADEOFFS.md](../design/TRADEOFFS.md)
