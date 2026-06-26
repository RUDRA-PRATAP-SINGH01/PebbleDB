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

`TestFlushNeverAbandonsQueueEntry` — close manifest, force flush retries, verify the queue entry is never dropped (only successful flush removes it).

## WAL / disk faults

| Test | Invariant |
|------|-----------|
| `TestWALAppendErrorBlocksWrites` | WAL close blocks Put, Get still works |
| `TestSyncWaitsForInFlightBatch` | `Sync()` waits for batch flusher mid-fsync |

## Manifest / disk faults

- Corrupt `wal.flush` → `ErrCorruptWalFlushState`
- Orphan SST on disk → quarantine on open
- Malformed SST filename → skipped

## What I do not inject yet

- Kernel-level `ENOSPC` simulation (WAL close approximates append failure)
- Network partition (N/A — embedded)
- Clock skew
