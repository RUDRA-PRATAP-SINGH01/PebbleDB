# Benchmarks overview

Benchmarks live in `internal/db/bench_test.go`.

## Suites

| Benchmark | Measures |
|-----------|----------|
| `BenchmarkSequentialWrite` | Put throughput at 100k / 500k / M keys |
| `BenchmarkRandomRead` | Parallel Get (GOMAXPROCS=4) over 50k keys |
| `BenchmarkScanThroughput` | Range scans 1k / 10k / 50k keys |

## Run

```bash
go test -bench=. -benchmem -count=1 ./internal/db
```

Do not use `-race` for timing.

## Durability

Default write benchmarks use async group commit. `ops/sec` does not include per-key fsync unless batch thresholds force `awaitBatchPersist`. Details: [METHODOLOGY.md](METHODOLOGY.md).
