package db

import (
	"log"
	"os"
	"time"
)

func removeSSTPath(path string) error {
	var last error
	for attempt := 0; attempt < 5; attempt++ {
		err := os.Remove(path)
		if err == nil || os.IsNotExist(err) {
			return nil
		}
		last = err
		time.Sleep(10 * time.Millisecond)
	}
	return last
}

// removeOrphanSSTFiles deletes on-disk SSTables not listed in the manifest.
func (db *DB) removeOrphanSSTFiles() {
	if db.manifest == nil {
		return
	}
	existing, err := discoverSSTIDs(db.dir)
	if err != nil {
		return
	}
	live := make(map[uint64]struct{}, len(db.manifest.LiveIDs()))
	for _, id := range db.manifest.LiveIDs() {
		live[id] = struct{}{}
	}
	for _, id := range existing {
		if _, ok := live[id]; ok {
			continue
		}
		path := sstFilePath(db.dir, id)
		if err := removeSSTPath(path); err != nil {
			log.Printf("pebbledb: remove orphan SST %s: %v", path, err)
		}
	}
}
