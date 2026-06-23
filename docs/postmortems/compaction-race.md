# Context

CI started failing with the race detector after compaction and close changes. The failure was not a flaky test — it was a real concurrent access bug between `Get` and the compactor.

# Original Design

Compaction picked the two oldest `sstable.Reader` pointers from `db.sstables`, merged them, updated the slice, and called `Close()` on inputs immediately.

`Get` copied the `sstables` slice under `RLock`, then searched without holding `db.mu`.

# Why I Thought It Was Correct

I believed removing readers from `db.sstables` meant no new `Get` could find them. I underestimated in-flight reads that already held a `Reader` reference.

# Failure Symptoms

- `go test -race ./...` reported concurrent access in `sstable.Reader` block reads during compaction.
- Rare test failures on `TestGetSurvivesCompactionWithHeldRefs`.
- Windows-specific file errors when compaction deleted SST files still open for read.

# Investigation

The race detector stack always showed:

1. Goroutine A: `Get` → `Ref` → block read.
2. Goroutine B: compaction → `Close` / `Discard` on same reader.

Commit `cfbbf5a` (`fix(close,sstable): stop compaction before closing SST readers to fix -race CI`) documents the fix. Related hardening in `0b2baf0`.

# Root Cause

Reference counting existed but compaction did not wait for in-flight readers to finish before closing file handles. On Windows, an open handle also blocks rename/delete — compounding the bug.

# Fix

I introduced explicit reader lifecycle:

- `Ref()` / `Unref()` on `sstable.Reader`.
- `Get` and `Scan` Ref before releasing `db.mu`, Unref in defer.
- Compaction calls `Discard()` which marks close-pending and closes only when refs reach zero.
- `trackReader` / `discardAllReaders` during `Close`.
- Compaction stops picking readers that are no longer in the slice (`readersStillPresent`).

# Verification

- `TestGetSurvivesCompactionWithHeldRefs`
- `go test -race -count=1 -shuffle=on ./...` in CI (ubuntu + macos)
- `get_test.go` — `TestLookupSSTReadersSkipsClosed`

# Lessons Learned

- In an LSM, **immutable** does not mean **unreferenced**. Readers can outlive index updates.
- I run race CI on every push now; Windows file locking makes these bugs obvious.
