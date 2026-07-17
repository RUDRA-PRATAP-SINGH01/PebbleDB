# Acceptance Testing Framework (ATF)

ATF proves PebbleDB crash recovery with a real process boundary and a durable oracle.

## Pipeline

1. Parent reserves resources and allocates an isolated temp directory.
2. Child writes a deterministic dataset via `LogicalWriter`.
3. Child persists `expected_state.json` (fsynced) **before** any crash.
4. Child calls `ForceMemtableFlush()` so engine `maybeCrash(PEBBLEDB_CRASH_AT)` can fire (`os.Exit(2)`).
5. Child clears `PEBBLEDB_CRASH_AT` before `Close()` so shutdown flush cannot re-crash.
6. Parent loads the oracle, reopens the DB, runs Get + Scan verifiers, then reopens 3× for idempotency.
7. `StatusPass` is returned **only** if all verifiers succeed. Failed runs retain the directory when configured.

## Child environment

| Variable | Meaning |
|----------|---------|
| `PEBBLEDB_CHILD_PROCESS=1` | Enter child main from test `init` |
| `PEBBLEDB_TEST_DIR` | Database directory |
| `PEBBLEDB_CRASH_AT` | Engine crash point (e.g. `flush_after_manifest`) |
| `PEBBLEDB_SEED` / `PEBBLEDB_KEY_COUNT` | Dataset controls |
| `PEBBLEDB_FORCE_FLUSH` | `0` disables flush (default force on) |

## Merge bar

A scenario must demonstrate: crash exit 2 → reopen → every live/tombstone key correct → scan ordered/unique → idempotent reopen.
