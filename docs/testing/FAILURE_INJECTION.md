# Failure injection

Beyond crash exits, I inject failures in tests to verify error propagation and queue behavior.

## Background error injection

Tests call `setBackgroundErr(op, err)` on `*DB` to simulate WAL, flush, or compaction failures without mocking the filesystem.

| Test | Invariant |
|------|-----------|
| `TestWalBackgroundErrorBlocksWritesOnly` | Put fails, Get succeeds |
| `TestFlushErrorBlocksWrites` | Put fails after flush error |
| `TestGetAllowedDuringBackgroundError` | Reads continue |

## Close failure injection

| Test | Technique |
|------|-----------|
| `TestCloseShutsDownWorkersOnWalSizeError` | Close WAL before `Close()` |
| `TestCloseIncompleteWhenWalSizeFails` | Expect `ErrCloseIncomplete` |
| `TestWalAppendFailurePreservesPendingBatch` | WAL closed mid-batch |

## Flush queue injection

`TestFlushRetryCapUnblocksQueue` — close manifest, force flush retries, verify queue drains after retry cap (`maxFlushRetries`).

## Manifest / disk faults

- Corrupt `wal.flush` → `ErrCorruptWalFlushState`
- Orphan SST on disk → quarantine on open
- Malformed SST filename → skipped

## What I do not inject yet

- `ioctl` disk full simulation
- Network partition (N/A — embedded)
- Clock skew

## Related

- [CRASH_TESTING.md](CRASH_TESTING.md)
- [../postmortems/shutdown-ordering.md](../postmortems/shutdown-ordering.md)
