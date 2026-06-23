# Benchmarks

Benchmark documentation moved to [docs/benchmarks/](docs/benchmarks/).

- [Overview](docs/benchmarks/BENCHMARKS.md)
- [Methodology](docs/benchmarks/METHODOLOGY.md)
- [Results template](docs/benchmarks/RESULTS.md)

Quick run:

```bash
go test -bench=. -benchmem -count=1 ./internal/db
```
