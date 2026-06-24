# Lessons learned

Notes from building PebbleDB.

## Durability ordering is the product

Durability boundaries:

- WAL fsync → durable log record
- manifest fsync → durable live SST set
- wal.flush → replay range metadata

Unnamed boundaries are untested boundaries.

## Test crashes, not just clean shutdown

Subprocess crash injection (`PEBBLEDB_CRASH_AT`) found bugs unit tests missed. `Close()` does not exercise crash paths.

## Race CI

`-race` failures (compaction vs `Get`, scan vs write) were real bugs. CI runs `-race -shuffle=on` on Linux/macOS.

## Windows file locking

Rename/delete with open handles fails differently than on Linux. Close handles before manifest truncate, WAL truncate, SST delete.

## API return values imply contracts

`Put` returning `nil` under async group commit is valid internally; callers need `Sync()` for durability.

## Manifest before memory

Manifest fsync before in-memory SST swap eliminated post-crash divergence between metadata and process state.

## Immutability ≠ safe to close

SST readers need refcounts. Removal from `db.sstables` is not destruction.

## Scan isolation has a memory cost

Snapshot copy unblocked writes. MVCC would be the production approach. Large memtables make scan expensive.

## Do not glob the data directory for truth

Directory listing is recovery input, not authority. Manifest is authority.

## Quarantine beats delete

Unknown SST files go to `quarantine/` for inspection, not immediate deletion.

## Scope

No replication or SQL — recovery and compaction shipped because scope stayed narrow.
