// Package interfaces defines the behavioral contracts of the ATF.
package interfaces

// LogicalWriter defines an abstract write pipeline interface.
// It decouples the dataset generators from the underlying PebbleDB instance.
type LogicalWriter interface {
	// Put writes a key-value pair to the database.
	Put(key, value []byte) error

	// Delete registers a tombstone for a key.
	Delete(key []byte) error

	// Flush forces in-memory writes to be enqueued for physical storage.
	Flush() error

	// Sync blocks until all written data is fsynced to disk.
	Sync() error

	// Close terminates the writer and releases resources.
	Close() error
}
