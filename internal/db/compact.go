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
	w, err := sstable.NewWriter(path, defaultBlockSize, expectedEntries)
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

	if err := db.manifest.AppendSetFileSet([]uint64{id}); err != nil {
		merged.Close()
		os.Remove(path)
		return err
	}

	oldPaths := make([]string, len(inputs))
	for i, r := range inputs {
		oldPaths[i] = r.Path()
		r.Close()
	}

	db.mu.Lock()
	db.sstables = []*sstable.Reader{merged}
	db.mu.Unlock()

	for _, p := range oldPaths {
		if p != "" {
			os.Remove(p)
		}
	}
	return nil
}
