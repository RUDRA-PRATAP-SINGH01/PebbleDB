package db

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/sstable"
)

const defaultCompactionThreshold = 4

func (db *DB) maybeCompact() {
	db.mu.Lock()
	if db.compactionThreshold <= 0 || len(db.sstables) < db.compactionThreshold {
		db.mu.Unlock()
		return
	}
	inputs := append([]*sstable.Reader(nil), db.sstables...)
	db.mu.Unlock()

	if err := db.compactSSTables(inputs); err != nil {
		db.setBackgroundErr("compaction", err)
		return
	}
	db.clearBackgroundErr()
}

func (db *DB) compactSSTables(inputs []*sstable.Reader) error {
	if len(inputs) < 2 {
		return nil
	}

	id := atomic.AddUint64(&db.nextSSTID, 1)
	path := filepath.Join(db.dir, fmt.Sprintf("sst_%08d.sst", id))

	expectedEntries := uint(1024)
	w, err := sstable.NewWriterWithBloom(path, defaultBlockSize, expectedEntries)
	if err != nil {
		return err
	}

	if err := sstable.MergeReaders(inputs, w); err != nil {
		w.Close()
		os.Remove(path)
		return err
	}
	if err := w.Close(); err != nil {
		os.Remove(path)
		return err
	}

	merged, err := sstable.OpenReader(path)
	if err != nil {
		os.Remove(path)
		return err
	}

	oldPaths := make([]string, len(inputs))
	for i, r := range inputs {
		oldPaths[i] = r.Path()
		r.Close()
	}

	db.mu.Lock()
	if len(inputs) == len(db.sstables) {
		db.sstables = []*sstable.Reader{merged}
	} else {
		db.sstables = append(db.sstables, merged)
	}
	db.mu.Unlock()

	for _, p := range oldPaths {
		if p != "" {
			os.Remove(p)
		}
	}
	return nil
}
