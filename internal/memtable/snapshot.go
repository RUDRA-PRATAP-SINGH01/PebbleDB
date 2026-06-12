package memtable

// SnapshotEntry is a point-in-time copy of one skip list entry.
type SnapshotEntry struct {
	Key       []byte
	Value     []byte
	Tombstone bool
}

// Snapshot returns a copy of all entries under a brief read lock.
// The returned slice is safe to iterate without holding any lock.
func (sl *SkipList) Snapshot() []SnapshotEntry {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	var out []SnapshotEntry
	for x := sl.head.next[0]; x != nil; x = x.next[0] {
		e := SnapshotEntry{
			Key:       append([]byte(nil), x.key...),
			Tombstone: x.tombstone,
		}
		if !x.tombstone {
			e.Value = append([]byte(nil), x.value...)
		}
		out = append(out, e)
	}
	return out
}
