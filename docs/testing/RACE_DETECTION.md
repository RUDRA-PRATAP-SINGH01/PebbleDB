# Race detection

I treat the Go race detector as a concurrency linter for storage code.

## CI configuration

```yaml
go test -race -count=1 -shuffle=on ./...
```

`-shuffle=on` reorders tests to expose ordering assumptions in flush/compaction integration tests.

## Bugs the race detector found

| Symptom | Root cause | Fix commit area |
|---------|------------|-----------------|
| Concurrent SST block read + Close | Compaction closed readers with active refs | `cfbbf5a` |
| Scan vs Put | Long-held memtable lock | snapshot copy |
| Close vs background workers | Tear-down while flusher active | `1336b21`, `505578a` |

## Patterns I use to stay race-clean

1. **Brief `db.mu` holds** — copy pointers, Ref SST readers, release.
2. **No IO under `db.mu`** — SST reads happen after RUnlock.
3. **`readersStillPresent`** — compaction validates picks after merge.
4. **Channel close only from owner** — `Close` closes `flushCh` after drain.

## Local workflow

```bash
go test ./... -race -count=1
```

Race builds are slower (~10×). I run them before push, not on every edit.

## What race detector does not catch

- Durability ordering bugs (need crash tests)
- Logical recovery errors (need replay assertions)
- File system atomicity assumptions on NFS
