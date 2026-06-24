# Contributing

PebbleDB is an educational LSM storage engine. Contributions should preserve durability ordering, pass CI, and include tests for behavior changes.

## Prerequisites

- Go 1.23.4 (see `go.mod`)
- `golangci-lint` (CI uses `golangci-lint-action`)

## Build

```bash
go build -o bin/pebbledb ./cmd/pebbledb
```

On Windows: `bin\pebbledb.exe` or `go build -o pebbledb.exe ./cmd/pebbledb`

## Test

```bash
go test ./...
go test -race -count=1 -shuffle=on ./...
go test ./internal/db -run Crash -v
```

Matches CI: see [.github/workflows/ci.yml](.github/workflows/ci.yml).

## Lint

```bash
go vet ./...
golangci-lint run ./...
```

## Benchmarks

```bash
go test ./internal/db -run=NonExistent -bench=. -benchmem -count=1
```

See [docs/benchmarks/](docs/benchmarks/).

## Documentation

- Architecture: [docs/architecture/](docs/architecture/)
- Correctness invariants: [docs/design/INVARIANTS.md](docs/design/INVARIANTS.md)
- Testing: [docs/testing/TESTING_STRATEGY.md](docs/testing/TESTING_STRATEGY.md)

Do not commit generated artifacts (`coverage.out`, `bench-results.txt`, local `data/` directories, binaries under `bin/`).

## Pull requests

- Keep changes focused; avoid unrelated refactors.
- Run `go test -race ./...` before opening a PR.
- Link to relevant docs or postmortems when changing durability or recovery paths.
