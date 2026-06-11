package iterator

// Iterator walks key-value entries in sorted order.
type Iterator interface {
	Valid() bool
	Next() error
	Key() []byte
	Value() []byte
	IsTombstone() bool
	Seek(key []byte) error
	Close() error
}
