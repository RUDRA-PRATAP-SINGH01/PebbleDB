# Read path

`Get` is a layered lookup: newest wins. I optimized for correctness first (tombstones, pending batch, compaction races), then bloom filters to skip cold SST files.

## Lookup order

```
1. active memtable
2. pendingFlush (newest → oldest)
3. pendingBatch (records not yet applied to memtable)
4. sstables (newest → oldest), bloom-filtered
```

```mermaid
flowchart TD
    START["Get(key)"] --> RLOCK["db.mu RLock"]
    RLOCK --> ACTIVE{"active memtable?"}
    ACTIVE -->|hit value| RET1["return value"]
    ACTIVE -->|tombstone| NF1["ErrNotFound"]
    ACTIVE -->|miss| PEND{"pendingFlush newest→oldest"}
    PEND -->|hit| RET2["return value"]
    PEND -->|tombstone| NF2["ErrNotFound"]
    PEND -->|miss| PENDING{"pendingBatch (unflushed WAL)?"}
    PENDING -->|hit| RET3["return value"]
    PENDING -->|miss| REF["copy sstables + Ref readers"]
    REF --> RUNLOCK["db.mu RUnlock"]
    RUNLOCK --> SST{"each SST newest→oldest"}
    SST --> BLOOM{"bloom MayContain?"}
    BLOOM -->|no| SST
    BLOOM -->|yes| BLOCK["index → block search"]
    BLOCK -->|found| RET4["return value"]
    BLOCK -->|tombstone| NF3["ErrNotFound"]
    BLOCK -->|miss| SST
    SST -->|exhausted| NF4["ErrNotFound"]
```

Source: [../diagrams/read-path.mmd](../diagrams/read-path.mmd)

## Why I check pendingBatch

With group commit, a record can be in `pendingBatch` after `Put` returns but before memtable apply. `Get` must see it or I would violate read-your-writes within a process.

## SST search

Under brief `RLock`:

1. Copy `sstables` slice pointer (also published via `atomic.Pointer` for some paths).
2. `Ref()` each reader I will consult.

After `RUnlock`:

1. Walk newest → oldest.
2. `bloom.MayContain(key)` — skip file on definite miss.
3. Binary search index block → load data block → scan entries.

`Unref()` in defer when `Get` returns.

## Tombstones

Newest tombstone → `ErrNotFound`. Older values hidden by tombstone in a newer layer are never visible. Compaction keeps tombstones until keys are dropped from output by merge semantics.

## Bloom filter role

I added blooms in commit `ec4cee5` when linear SST scans became measurable. False positives cost one extra block read; false negatives never occur by construction.

## Performance work

Commit `052812d` overhauled the read path:

- LRU block cache (`hashicorp/golang-lru/v2`) — optional via `BlockCacheSize`
- `sstablesSnap` atomic snapshot to shorten lock hold
- Bloom before index probe

I still accept higher read amplification than leveled compaction engines — my compaction policy is naive.

## Background errors

`Get` ignores background errors. I made that choice so a stuck flush does not brick reads of durable data. See [CONCURRENCY_MODEL.md](CONCURRENCY_MODEL.md).

## Related

- [SSTABLE_DESIGN.md](SSTABLE_DESIGN.md)
- [../postmortems/compaction-race.md](../postmortems/compaction-race.md)
- [../postmortems/reader-lifecycle.md](../postmortems/reader-lifecycle.md)
