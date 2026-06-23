# Lessons learned

Patterns I would reuse, and mistakes I would not repeat, after building PebbleDB from scratch.

## Durability ordering is the product

I used to think the WAL was the database. Now I think in **boundaries**:

- WAL fsync → durable log record
- manifest fsync → durable live SST set
- wal.flush → replay range metadata

If I cannot name the boundary, I cannot test crash recovery.

## Test crashes, not just clean shutdown

Subprocess crash injection (`PEBBLEDB_CRASH_AT`) found bugs unit tests missed. Clean `Close()` lies about production.

## Race CI is non-negotiable

Every `-race` failure I fixed (compaction vs `Get`, scan vs write) was a real bug. I run `-race -shuffle=on` on Linux/macOS CI.

## Windows file locking is a feature detector

Rename/delete while handles are open fails differently than on Linux. I close handles before rename on manifest truncate, WAL truncate, SST delete.

## API return values imply contracts

`Put` returning `nil` with async group commit is correct internally and misleading externally. I added `Sync()` rather than pretending.

## Manifest before memory

One ordering rule eliminated whole classes of post-crash divergence. I check it on every new background operation.

## Immutability ≠ safe to close

SST readers need refcounts. Index removal is not destruction.

## Scan isolation has a memory cost

Snapshot copy was the right first fix. MVCC would be the production fix. Large memtables make scan expensive.

## Do not glob the data directory for truth

Directory listing is recovery input, not authority. Manifest is authority.

## Quarantine beats delete

When I am unsure if a file is garbage, I move it aside. Debugging storage without artifacts is miserable.

## Scope control enabled depth

Saying no to replication and SQL let me finish recovery. Breadth without depth would have produced a demo, not an engine.
