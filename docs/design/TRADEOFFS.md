# Tradeoffs

Every storage engine optimizes for something. PebbleDB optimizes for **understandable durability** over feature breadth and peak performance.

| Decision | Benefit | Cost I accept |
|----------|---------|----------------|
| LSM architecture | Sequential writes, immutable files | Compaction required; read amplification |
| Skip list memtable | Simple concurrent inserts | Approximate `Size()` tracking |
| Single writer lock | Easier reasoning | No parallel `Put` throughput |
| Group commit (default) | High write throughput | `Put` may return before fsync |
| WAL sync before memtable apply | Replay correctness | Latency when sync required |
| Manifest fsync per flush/compaction | Crash-consistent live set | Extra fsync latency |
| wal.flush sidecar | Correct replay offset | Extra file; crash windows to test |
| Oldest-2 compaction | Simple implementation | Suboptimal write/read amp vs leveled |
| Per-SST bloom | Cheap negative lookups | Space + false positive block reads |
| Tombstones in SST | Correct delete semantics | Space until compaction merges |
| Scan snapshot copy | Writes not blocked during scan | Memory spike; stale iterator view |
| Block cache (optional) | Hot block reuse | Memory; invalidation complexity |
| Scoped background errors | Reads during partial failure | Caller must handle write errors |
| No network server | Smaller attack surface | CLI/library only |
| No MVCC | Simpler keys | No snapshot isolation across time |
| Quarantine vs delete | Debuggable recovery | Disk clutter until manual cleanup |
| Close timeout (30s) | Bounded shutdown | `ErrCloseIncomplete` leaves handles open |

## Durability vs latency

I expose three levels intentionally:

1. **Async Put** — fast, may lose last milliseconds on crash.
2. **`Sync()`** — barrier for prior async writes.
3. **`SyncWrites`** — per-op fsync.

I document this in CLI help and benchmarks. Misinterpreting default `Put` as durable was a lesson from my own early misuse.

## Teaching vs production

I would not ship PebbleDB as a multi-tenant production database without:

- Leveled or tiered compaction policy
- MVCC or snapshot timestamps
- Rigorous fuzzing / Jepsen-style testing
- Operational metrics and backpressure

I am honest about that limit in the root README.

## Related

- [DECISIONS.md](DECISIONS.md)
- [LESSONS_LEARNED.md](LESSONS_LEARNED.md)
