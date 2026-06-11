package db

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/manifest"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/sstable"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"
)

const (
	defaultMemtableSize = 4 << 20 // 4MB
	defaultBlockSize    = 4096
)

var sstFilePattern = regexp.MustCompile(`^sst_(\d{8})\.sst$`)

// DB is the main database handle.
type DB struct {
	mu                  sync.RWMutex
	dir                 string
	active              *memtable.SkipList
	immutable           *memtable.SkipList
	sstables            []*sstable.Reader
	wal                 *wal.WAL
	manifest            *manifest.Log
	closed              bool
	flushCh          chan struct{}
	flushDone        chan struct{}
	compactCh        chan struct{}
	compactDone      chan struct{}
	compactMu        sync.Mutex
	nextSSTID        uint64
	memtableSize     int64
	compactThreshold int
	walLimits           wal.ReplayLimits
	walFreezeOffset     int64
	bgErr               atomic.Pointer[BackgroundError]
}

// Options control database behaviour.
type Options struct {
	Dir                   string
	MemtableSize int64 // bytes; 0 uses default (4MB)
	CompactionThreshold   int   // SSTable count before compaction; 0 uses default (4)
	WALReplayLimits       wal.ReplayLimits
}

// Open opens or creates a database at the given directory.
func Open(opts Options) (*DB, error) {
	if err := os.MkdirAll(opts.Dir, 0755); err != nil {
		return nil, err
	}

	memtableSize := opts.MemtableSize
	if memtableSize <= 0 {
		memtableSize = defaultMemtableSize
	}

	compactThreshold := opts.CompactionThreshold
	if compactThreshold == 0 {
		compactThreshold = defaultCompactThreshold
	}

	walLimits := opts.WALReplayLimits.WithDefaults()

	db := &DB{
		dir:                 opts.Dir,
		active:              memtable.NewSkipList(),
		memtableSize:        memtableSize,
		compactThreshold: compactThreshold,
		walLimits:        walLimits,
		flushCh:          make(chan struct{}, 8),
		flushDone:        make(chan struct{}),
		compactCh:        make(chan struct{}, 8),
		compactDone:      make(chan struct{}),
	}

	m, err := manifest.Open(opts.Dir)
	if err != nil {
		return nil, err
	}
	db.manifest = m

	existing, err := discoverSSTIDs(opts.Dir)
	if err != nil {
		db.manifest.Close()
		return nil, err
	}
	if err := db.manifest.BootstrapIfEmpty(existing); err != nil {
		db.manifest.Close()
		return nil, err
	}

	walPath := filepath.Join(opts.Dir, "wal.log")
	w, err := wal.OpenWithLimits(walPath, walLimits)
	if err != nil {
		db.manifest.Close()
		return nil, err
	}
	db.wal = w

	err = wal.ReplayWithLimits(walPath, walLimits, func(rec wal.Record) error {
		if rec.Tombstone {
			db.active.Delete(rec.Key)
		} else {
			db.active.Put(rec.Key, rec.Value)
		}
		return nil
	})
	if err != nil {
		db.wal.Close()
		db.manifest.Close()
		return nil, err
	}

	if err := db.loadSSTables(); err != nil {
		db.wal.Close()
		db.manifest.Close()
		return nil, err
	}

	go db.flusher()
	go db.compactor()
	db.maybeTriggerCompaction()
	return db, nil
}

func discoverSSTIDs(dir string) ([]uint64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var ids []uint64
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
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func sstFilePath(dir string, id uint64) string {
	return filepath.Join(dir, fmt.Sprintf("sst_%08d.sst", id))
}

func (db *DB) loadSSTables() error {
	for _, id := range db.manifest.LiveIDs() {
		path := sstFilePath(db.dir, id)
		r, err := sstable.OpenReader(path)
		if err != nil {
			return err
		}
		db.sstables = append(db.sstables, r)
		if id > db.nextSSTID {
			db.nextSSTID = id
		}
	}
	return nil
}

func (db *DB) setBackgroundErr(op string, err error) {
	if err == nil {
		return
	}
	db.bgErr.Store(&BackgroundError{Op: op, Err: err})
}

func (db *DB) clearBackgroundErr() {
	db.bgErr.Store(nil)
}

func (db *DB) backgroundErr() error {
	if p := db.bgErr.Load(); p != nil {
		return p
	}
	return nil
}

// BackgroundError returns the most recent background flush or compaction error, if any.
func (db *DB) BackgroundError() error {
	return db.backgroundErr()
}

