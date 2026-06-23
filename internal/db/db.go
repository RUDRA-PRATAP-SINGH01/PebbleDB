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
	"time"

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
	active       *memtable.SkipList
	pendingFlush []flushQueueEntry
	sstables     []*sstable.Reader
	wal                 *wal.WAL
	manifest            *manifest.Log
	closed              bool
	flushCh          chan struct{}
	flushDone        chan struct{}
	compactCh        chan struct{}
	compactDone      chan struct{}
	compactMu        sync.Mutex
	readersMu        sync.Mutex
	allReaders       []*sstable.Reader
	nextSSTID        uint64
	memtableSize     int64
	compactThreshold int
	walLimits  wal.ReplayLimits
	blockCache *sstable.BlockCache
	bgErr      atomic.Pointer[BackgroundError]

	// Group commit: batch WAL appends and fsync once per batch.
	pendingBatch   []wal.Record
	batchSizeBytes int
	batchTimer     *time.Timer
	batchFlushCh   chan struct{}
	batchStop      chan struct{}
	batchDone      chan struct{}
}

type flushQueueEntry struct {
	mem       *memtable.SkipList
	walCutoff int64
}

// Options control database behaviour.
type Options struct {
	Dir                   string
	MemtableSize int64 // bytes; 0 uses default (4MB)
	CompactionThreshold   int   // SSTable count before compaction; 0 uses default (4)
	WALReplayLimits       wal.ReplayLimits
	BlockCacheSize        int64 // bytes; 0 uses default (32 MiB); negative disables caching
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

	var blockCache *sstable.BlockCache
	if opts.BlockCacheSize < 0 {
		blockCache = nil
	} else {
		blockCache = sstable.NewBlockCache(int(opts.BlockCacheSize))
	}

	db := &DB{
		dir:                 opts.Dir,
		active:              memtable.NewSkipList(),
		memtableSize:        memtableSize,
		compactThreshold: compactThreshold,
		walLimits:        walLimits,
		blockCache:       blockCache,
		flushCh:          make(chan struct{}, 8),
		flushDone:        make(chan struct{}),
		compactCh:        make(chan struct{}, 8),
		compactDone:      make(chan struct{}),
		batchFlushCh:     make(chan struct{}, 1),
		batchStop:        make(chan struct{}),
		batchDone:        make(chan struct{}),
		pendingBatch:     make([]wal.Record, 0, batchMaxRecords),
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

	if err := db.loadSSTables(); err != nil {
		db.manifest.Close()
		return nil, err
	}
	db.removeOrphanSSTFiles()

	walPath := filepath.Join(opts.Dir, "wal.log")
	replayStart, err := db.walReplayStartOffset()
	if err != nil {
		db.closeLoadedReaders()
		db.manifest.Close()
		return nil, err
	}
	_, err = wal.ReplayFromWithRecovery(walPath, walLimits, replayStart, func(rec wal.Record) error {
		if rec.Tombstone {
			db.active.Delete(rec.Key)
		} else {
			db.active.Put(rec.Key, rec.Value)
		}
		return nil
	})
	if err != nil {
		db.closeLoadedReaders()
		db.manifest.Close()
		return nil, err
	}

	w, err := wal.OpenWithLimits(walPath, walLimits)
	if err != nil {
		db.closeLoadedReaders()
		db.manifest.Close()
		return nil, err
	}
	db.wal = w

	go db.batchFlusher()
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

func (db *DB) closeLoadedReaders() {
	for _, r := range db.sstables {
		r.Close()
	}
	db.sstables = nil
}

func (db *DB) loadSSTables() error {
	for _, id := range db.manifest.LiveIDs() {
		path := sstFilePath(db.dir, id)
		r, err := sstable.OpenReader(path, db.blockCache)
		if err != nil {
			return err
		}
		db.sstables = append(db.sstables, r)
		db.trackReader(r)
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

func (db *DB) clearBackgroundErrOp(op string) {
	if p := db.bgErr.Load(); p != nil && p.Op == op {
		db.bgErr.Store(nil)
	}
}

func (db *DB) trackReader(r *sstable.Reader) {
	if r == nil {
		return
	}
	db.readersMu.Lock()
	db.allReaders = append(db.allReaders, r)
	db.readersMu.Unlock()
}

func (db *DB) discardAllReaders() {
	db.readersMu.Lock()
	for _, r := range db.allReaders {
		_ = r.Close()
	}
	db.allReaders = nil
	db.readersMu.Unlock()
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

