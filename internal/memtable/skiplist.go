package memtable

import (
	"math/rand"
	"sync"
	"time"
)

const (
	maxHeight = 20
	p         = 0.25 // probability for level promotion
)

// SkipList is a concurrent‑safe in‑memory sorted map.
type SkipList struct {
	mu     sync.RWMutex
	head   *node
	height int   // current max level
	length int   // number of entries (including tombstones)
	size   int64 // approximate byte size (used only for flush threshold)
	rng    *rand.Rand
}

// NewSkipList creates a new skip list.
func NewSkipList() *SkipList {
	src := rand.NewSource(time.Now().UnixNano())
	return &SkipList{
		head: &node{
			next: make([]*node, maxHeight),
		},
		height: 1,
		rng:    rand.New(src),
	}
}

func (sl *SkipList) randomHeight() int {
	h := 1
	for sl.rng.Float64() < p && h < maxHeight {
		h++
	}
	return h
}

// Put inserts or updates a key‑value pair.
func (sl *SkipList) Put(key, value []byte) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	update := make([]*node, maxHeight)
	x := sl.head
	for i := sl.height - 1; i >= 0; i-- {
		for x.next[i] != nil && less(x.next[i].key, key) {
			x = x.next[i]
		}
		update[i] = x
	}

	x = x.next[0]
	if x != nil && equal(x.key, key) {
		// update existing – copy both key and value to avoid aliasing
		oldSize := int64(len(x.key) + len(x.value) + 8)
		newKey := append([]byte(nil), key...)
		newVal := append([]byte(nil), value...)
		sl.size -= oldSize
		x.key = newKey
		x.value = newVal
		x.tombstone = false
		sl.size += int64(len(newKey) + len(newVal) + 8)
		return
	}

	height := sl.randomHeight()
	if height > sl.height {
		for i := sl.height; i < height; i++ {
			update[i] = sl.head
		}
		sl.height = height
	}

	// copy key and value to avoid aliasing
	keyCopy := append([]byte(nil), key...)
	valCopy := append([]byte(nil), value...)

	newNode := &node{
		key:   keyCopy,
		value: valCopy,
		next:  make([]*node, height),
	}
	for i := 0; i < height; i++ {
		newNode.next[i] = update[i].next[i]
		update[i].next[i] = newNode
	}
	sl.length++
	sl.size += int64(len(keyCopy) + len(valCopy) + 8)
}

// Get retrieves a value and tombstone flag. Returns a copy of the value
// to prevent caller mutation. If the key is a tombstone, found=true and
// isTombstone=true, value=nil.
func (sl *SkipList) Get(key []byte) (value []byte, found bool, isTombstone bool) {
	sl.mu.RLock()
	defer sl.mu.RUnlock()

	x := sl.head
	for i := sl.height - 1; i >= 0; i-- {
		for x.next[i] != nil && less(x.next[i].key, key) {
			x = x.next[i]
		}
	}
	x = x.next[0]
	if x != nil && equal(x.key, key) {
		if x.tombstone {
			return nil, true, true
		}
		// copy to prevent caller mutation
		valCopy := append([]byte(nil), x.value...)
		return valCopy, true, false
	}
	return nil, false, false
}

// Delete marks a key as deleted (tombstone).
func (sl *SkipList) Delete(key []byte) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	update := make([]*node, maxHeight)
	x := sl.head
	for i := sl.height - 1; i >= 0; i-- {
		for x.next[i] != nil && less(x.next[i].key, key) {
			x = x.next[i]
		}
		update[i] = x
	}
	x = x.next[0]
	if x != nil && equal(x.key, key) {
		if !x.tombstone {
			// remove only the value size; key remains for tombstone
			sl.size -= int64(len(x.value))
			x.tombstone = true
			x.value = nil
		}
		return
	}
	// insert a tombstone – copy key to avoid aliasing
	height := sl.randomHeight()
	if height > sl.height {
		for i := sl.height; i < height; i++ {
			update[i] = sl.head
		}
		sl.height = height
	}
	keyCopy := append([]byte(nil), key...)
	newNode := &node{
		key:       keyCopy,
		value:     nil,
		tombstone: true,
		next:      make([]*node, height),
	}
	for i := 0; i < height; i++ {
		newNode.next[i] = update[i].next[i]
		update[i].next[i] = newNode
	}
	sl.length++
	sl.size += int64(len(keyCopy) + 8) // tombstone overhead (key + approx. node overhead)
}

// Size returns approximate memory usage in bytes.
// It is only used for flush threshold decisions and is not exact.
func (sl *SkipList) Size() int64 {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return sl.size
}

// Len returns number of entries (including tombstones).
func (sl *SkipList) Len() int {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return sl.length
}

// less compares two byte slices lexicographically.
func less(a, b []byte) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
