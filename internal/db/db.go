package db

import (
	"path/filepath"
	"sync"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"
)

// DB is the main database handle.
type DB struct {
	mu       sync.RWMutex
	dir      string
	active   *memtable.SkipList // current memtable
	wal      *wal.WAL
	closed   bool
}

// Options control database behaviour.
type Options struct {
	Dir string // data directory (where WAL and SSTables will live)
}

// Open opens or creates a database at the given directory.
func Open(opts Options) (*DB, error) {
	db := &DB{
		dir:    opts.Dir,
		active: memtable.NewSkipList(),
	}

	// Open WAL
	walPath := filepath.Join(opts.Dir, "wal.log")
	w, err := wal.Open(walPath)
	if err != nil {
		return nil, err
	}
	db.wal = w

	// Replay WAL into memtable
	err = wal.Replay(walPath, func(rec wal.Record) error {
		if rec.Tombstone {
			db.active.Delete(rec.Key)
		} else {
			db.active.Put(rec.Key, rec.Value)
		}
		return nil
	})
	if err != nil {
		db.wal.Close()
		return nil, err
	}

	return db, nil
}

// Close flushes and closes the database.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return nil
	}
	db.closed = true
	if db.wal != nil {
		db.wal.Sync()
		db.wal.Close()
	}
	return nil
}