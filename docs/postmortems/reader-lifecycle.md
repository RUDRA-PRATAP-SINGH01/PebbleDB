# Context

SSTable readers wrap `os.File` handles and block caches. I needed them to survive concurrent `Get` during compaction without leaking handles on shutdown — especially on Windows.

# Original Design

Each `sstable.Reader` opened one file handle. Compaction and `Close` called `Close()` directly when an SST left the live set.

# Why I Thought It Was Correct

Once an SST was compacted away, I assumed no code path would read it again.

# Failure Symptoms

- `ACCESS_DENIED` / `ERROR_SHARING_VIOLATION` on Windows when deleting SST files during compaction.
- Race detector: concurrent read in `ReadBlock` vs `Close`.
- Handle leaks in long-running tests when compaction overlapped with scans.

# Investigation

I traced `Get` and `Scan` and found they could hold a reader pointer after compaction removed it from `db.sstables`. `Close()` during shutdown had the same problem with background compaction.

# Root Cause

No distinction between **logically removed** (not in live set) and **safe to close** (no in-flight IO).

# Fix

`sstable.Reader` now has:

- `refs atomic.Int32`
- `closePending atomic.Bool`
- `Ref()` increments; `Unref()` decrements and closes when zero if close pending.
- `Discard()` sets close pending; does not force-close active readers.

`db.trackReader` registers all readers for shutdown `discardAllReaders`.

Block cache keys include file path + offset (commit `0a7a5fa` area) so recycled file ids do not serve stale blocks.

# Verification

- Race CI on compaction + get tests.
- Manual Windows testing with flush/compaction loops.

# Lessons Learned

- File handle lifetime is a first-class API in embedded storage — not an implementation detail.
- I model reader lifecycle like Arc: drop from index ≠ destroy object.
