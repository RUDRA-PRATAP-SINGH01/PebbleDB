# PebbleDB Flush Crash Recovery Test Plan
## Verification Specification

I designed this test plan to certify how PebbleDB recovers from a sudden process crash during the flush pipeline. A flush is the critical operation where the database moves data from temporary computer memory into permanent filing cabinets on disk. If a crash occurs during this state transition, the database must recover without losing a single write that was already confirmed to the client.

Here is a visual map of the write pipeline showing exactly where the database can fail:

```
[User Write Request]
         │
         ▼
[Write Ahead Log]  =======>  [Active Memtable]
(Written to disk)            (Kept in volatile memory)
         │
         ▼
(When memtable is full)
         │
         ▼
[Frozen Memtable]  =======>  [Subprocess Spawned to Flush]
(Enqueued for flush)         (Writes data to a new SSTable file)
                                  │
                                  ▼
                             [Manifest Log]
                             (Registers SSTable as active)
                                  │
                                  ▼
                             [Checkpoint Created]
                             (Guides recovery starting offset)
                                  │
                                  ▼
                             [WAL Truncated]
                             (Old records cleared from logbook)
```

---

## 1. Overview and Goal

My goal is to verify database correctness by testing the most adversarial crash conditions. I want to terminate the database process abruptly at every stage of the flush sequence and check if it recovers perfectly when restarted.

This is a black box acceptance test. I do not want to test code functions in isolation. I want to inspect the filesystem state after the crash and verify that the database conforms to its structural and logical invariants from a client perspective.

---

## 2. Core Guarantees to Verify

When the database recovers from a crash, it must satisfy these fundamental contracts:

* No Confirmed Writes Lost: Any write that was confirmed to the client must survive the crash. The data must be readable via get and scan commands after recovery.
* Manifest Authority: The database must only load filing cabinet files that are officially registered in the manifest directory. If a file is on disk but not in the directory, it must be quarantined and ignored.
* Duplicate Suppression: If the write ahead log contains records that were already written to a filing cabinet, the recovery engine must not duplicate those keys during replay. The read path must resolve version overlaps, returning only the most recent version.
* Transactional Consistency: If a batch of writes was written, either all of those writes must be visible after recovery or none of them. We must never see a partially applied batch.

---

## 3. Crash Boundaries to Test

I have mapped out sixteen distinct points in the flush pipeline where a process termination could occur. The test suite must inject a crash at each of these locations:

1. After the memory scratchpad is frozen but before writing starts on disk.
2. After creating the new filing cabinet file on disk but before writing any data.
3. While writing the first block of data.
4. After writing all data blocks but before writing the index structure or file footer.
5. After the file is structurally complete but before it is fsynced to stable disk storage.
6. After the filing cabinet is fsynced but before a record is added to the manifest.
7. While appending the registration record to the manifest.
8. After the manifest record is written to the OS buffer but before it is fsynced.
9. Immediately after the manifest record is fsynced to disk (this is the primary durability boundary).
10. After the active memory lists are updated in the running process.
11. After the write ahead log checkpoint file is written.
12. While copying the remaining write ahead log tail to a temporary file.
13. After the temporary log tail file is fsynced but before renaming it to the canonical log file path.
14. Immediately after renaming the truncated log file.
15. After removing the temporary checkpoint file.
16. After removing the flush entry from the task queue.

---

## 4. Verification and Audit Strategy

For each of the sixteen crash points, the framework must execute a verification pipeline consisting of:

* Directory Audit: Checking the filesystem to ensure only registered database files are present. Any unregistered filing cabinets must be isolated in the quarantine directory.
* Manifest Audit: Parsing the manifest log independently to confirm the live file set matches the active directory contents.
* Checkpoint Audit: Confirming that if a checkpoint file is present, its target filing cabinet exists in the manifest. If not, the checkpoint must be discarded.
* Data Integrity Sweep: Running individual get operations on every written key, asserting that returned values match expected data and no key returns a stale version.
* Lexicographic Scan: Executing a full range scan to confirm that all keys are returned in sorted order, with zero duplicate entries.
* Idempotency Check: Closing and reopening the recovered database ten times to prove that recovery is a stable, deterministic function of the disk state.
