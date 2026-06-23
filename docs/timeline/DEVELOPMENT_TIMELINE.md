# Development timeline

Chronological record of how PebbleDB grew. Dates approximate from git history.

## 2026-06 — Foundation

| Period | Work |
|--------|------|
| Early Jun | Skip list memtable, WAL with CRC, basic Put/Get (`eee97de`) |
| Mid Jun | SSTable blocks, index, footer; tombstone support (`5a34ff1`, `5df52e0`) |
| Mid Jun | WAL truncate fixes for Windows (`ca28f73`, `78e8eb8`) |
| Mid Jun | Flush recovery bugs, WAL replay scope (`d0a4a0a`) |

## 2026-06 — LSM features

| Period | Work |
|--------|------|
| Mid Jun | Background flusher, bloom filters, SST-aware Get (`ec4cee5`) |
| Mid Jun | Compaction oldest-2, WAL size limits (`e65cf72`, `7590b2c`) |
| Mid Jun | Range scan, merge iterator, CLI (`05f073d`) |
| Mid Jun | Recovery/concurrency audit fixes (`7c420c7`, `054e6f7`) |

## 2026-06 — Hardening and performance

| Period | Work |
|--------|------|
| Late Jun | Group commit WAL batching (`01eef8e`) |
| Late Jun | Compaction/manifest atomicity (`0b2baf0`, `fd701a3`) |
| Late Jun | Close boundedness, WAL atomic truncate (`1336b21`) |
| Late Jun | Read path cache, async batch flusher (`052812d`) |
| Late Jun | Durability API: Sync, SyncWrites, LOCK file (`0a7a5fa`) |
| Late Jun | CLI hardening, gitignore fix, Unix flock CI fix (`f9833ad`–`95541a8`) |

## Engineering problems vs commits

I document **problems** in postmortems, not every commit:

| Problem | Doc |
|---------|-----|
| WAL replay duplicated flushed data | [wal-replay-bug.md](../postmortems/wal-replay-bug.md) |
| Manifest/memory ordering | [manifest-consistency.md](../postmortems/manifest-consistency.md) |
| Compaction vs Get race | [compaction-race.md](../postmortems/compaction-race.md) |
| Scan blocked writes | [scan-lock-contention.md](../postmortems/scan-lock-contention.md) |
| Close hung or tore down early | [shutdown-ordering.md](../postmortems/shutdown-ordering.md) |

## Related

- [MAJOR_MILESTONES.md](MAJOR_MILESTONES.md)
- [../design/EVOLUTION.md](../design/EVOLUTION.md)
