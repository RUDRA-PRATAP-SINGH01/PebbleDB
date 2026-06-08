package memtable

// node represents an element in the skip list
type node struct {
	key       []byte
	value     []byte
	tombstone bool
	next      []*node // pointers to next nodes at each level
}