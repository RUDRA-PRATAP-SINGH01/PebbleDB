# Diagrams

Mermaid sources for architecture docs. Render in GitHub, VS Code (Mermaid extension), or [mermaid.live](https://mermaid.live).

| File | Topic |
|------|-------|
| [architecture.mmd](architecture.mmd) | Layered system overview |
| [write-path.mmd](write-path.mmd) | Put / group commit sequence |
| [read-path.mmd](read-path.mmd) | Get layer walk |
| [flush.mmd](flush.mmd) | Memtable flush to SST |
| [wal-truncation.mmd](wal-truncation.mmd) | WAL truncate after flush |
| [manifest.mmd](manifest.mmd) | Manifest append and rotation |
| [compaction.mmd](compaction.mmd) | Oldest-2 compaction flow |
| [recovery.mmd](recovery.mmd) | `Open` sequence |
| [crash-recovery.mmd](crash-recovery.mmd) | `PEBBLEDB_CRASH_AT` test flow |
| [concurrency.mmd](concurrency.mmd) | Locks and worker model |
| [scan.mmd](scan.mmd) | Scan snapshot path |
| [shutdown.mmd](shutdown.mmd) | `Close` worker drain |
| [sstable-layout.mmd](sstable-layout.mmd) | On-disk SST block layout |

Embedded copies also appear in [docs/architecture/](../architecture/) and the root [README.md](../../README.md).
