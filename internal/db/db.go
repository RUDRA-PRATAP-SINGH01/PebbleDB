package db

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/sstable"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"
)

const (
	defaultMemtableSizeThreshold int64 = 4096
	defaultBlockSize                   = 4096
)

var sstFilePattern = regexp.MustCompile(`^sst_(\d{8})\.sst$`)

// DB is the main database handle.
type DB struct {
	mu                sync.RWMutex
	dir               string
	active            *memtable.SkipList
	immutable         *memtable.SkipList
	sstables          []*sstable.Reader
	wal               *wal.WAL
	closed            bool
	flushCh           chan struct{}
	flushDone         chan struct{}
	nextSSTID         uint64
	memtableThreshold int64
}

// Options control database behaviour.
type Options struct {
	Dir                   string
	MemtableSizeThreshold int64 // bytes; 0 uses default (4096)
}

// Open opens or creates a database at the given directory.
func Open(opts Options) (*DB, error) {
	if err := os.MkdirAll(opts.Dir, 0755); err != nil {
		return nil, err
	}

	threshold := opts.MemtableSizeThreshold
	if threshold <= 0 {
		threshold = defaultMemtableSizeThreshold
	}

	db := &DB{
		dir:               opts.Dir,
		active:            memtable.NewSkipList(),
		memtableThreshold: threshold,
		flushCh:           make(chan struct{}, 8),
		flushDone:         make(chan struct{}),
	}

	walPath := filepath.Join(opts.Dir, "wal.log")
	w, err := wal.Open(walPath)
	if err != nil {
		return nil, err
	}
	db.wal = w

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

	if err := db.loadSSTables(); err != nil {
		db.wal.Close()
		return nil, err
	}

	go db.flusher()
	return db, nil
}

func (db *DB) loadSSTables() error {
	entries, err := os.ReadDir(db.dir)
	if err != nil {
		return err
	}

	type sstFile struct {
		id   uint64
		path string
	}
	var files []sstFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := sstFilePattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		id, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			continue
		}
		files = append(files, sstFile{id: id, path: filepath.Join(db.dir, e.Name())})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].id < files[j].id })

	for _, f := range files {
		r, err := sstable.OpenReader(f.path)
		if err != nil {
			return err
		}
		db.sstables = append(db.sstables, r)
		if f.id > db.nextSSTID {
			db.nextSSTID = f.id
		}
	}
	return nil
}

// Close flushes pending data and closes the database.
func (db *DB) Close() error {
	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return nil
	}
	db.closed = true
	db.mu.Unlock()

	for {
		var needFlush bool
		db.mu.Lock()
		if db.immutable == nil && db.active.Len() == 0 {
			db.mu.Unlock()
			break
		}
		if db.immutable == nil && db.active.Len() > 0 {
			db.immutable = db.active
			db.active = memtable.NewSkipList()
			needFlush = true
		}
		db.mu.Unlock()

		if needFlush {
			db.flushCh <- struct{}{}
		}
		db.waitForImmutableDrain()
	}

	close(db.flushCh)
	<-db.flushDone

	db.mu.Lock()
	defer db.mu.Unlock()
	for _, r := range db.sstables {
		r.Close()
	}
	db.sstables = nil
	if db.wal != nil {
		db.wal.Sync()
		db.wal.Close()
		db.wal = nil
	}
	return nil
}

func (db *DB) waitForImmutableDrain() {
	for {
		db.mu.RLock()
		imm := db.immutable
		db.mu.RUnlock()
		if imm == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}
