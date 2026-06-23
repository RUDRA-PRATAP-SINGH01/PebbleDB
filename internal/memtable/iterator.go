package memtable

// SkipListIterator iterates over the skip list in order.
// It holds a read lock on the skip list until Close() is called.
// The caller MUST call Close() when finished (typically with defer).
// Failure to call Close() will block any future writes indefinitely.
type SkipListIterator struct {
	sl   *SkipList
	node *node
}

// Iterator returns a new iterator. The caller must call Close() when done.
func (sl *SkipList) Iterator() *SkipListIterator {
	sl.mu.RLock()
	return &SkipListIterator{
		sl:   sl,
		node: sl.head.next[0],
	}
}

// Valid returns true if the iterator is positioned at a valid entry.
func (it *SkipListIterator) Valid() bool {
	return it.node != nil
}

// Next advances the iterator to the next entry.
func (it *SkipListIterator) Next() {
	if it.node != nil {
		it.node = it.node.next[0]
	}
}

// Seek positions the iterator at the first entry with key >= target.
func (it *SkipListIterator) Seek(key []byte) {
	if it.sl == nil {
		it.node = nil
		return
	}
	if len(key) == 0 {
		it.node = it.sl.head.next[0]
		return
	}
	x := it.sl.head
	for i := it.sl.height - 1; i >= 0; i-- {
		for x.next[i] != nil && less(x.next[i].key, key) {
			x = x.next[i]
		}
	}
	it.node = x.next[0]
}

// Key returns a copy of the current key. Callers may mutate the returned slice
// without affecting the skip list.
func (it *SkipListIterator) Key() []byte {
	if it.node == nil {
		return nil
	}
	// defensive copy
	return append([]byte(nil), it.node.key...)
}

// Value returns a copy of the current value. If the entry is a tombstone,
// returns nil. Callers may mutate the returned slice without affecting the
// skip list.
func (it *SkipListIterator) Value() []byte {
	if it.node == nil {
		return nil
	}
	if it.node.tombstone {
		return nil
	}
	return append([]byte(nil), it.node.value...)
}

// IsTombstone returns true if the current entry is a deletion tombstone.
func (it *SkipListIterator) IsTombstone() bool {
	return it.node != nil && it.node.tombstone
}

// Close releases the read lock held by the iterator.
// Must be called exactly once. After Close, the iterator is unusable.
func (it *SkipListIterator) Close() {
	if it.sl != nil {
		it.sl.mu.RUnlock()
		it.sl = nil
	}
}
