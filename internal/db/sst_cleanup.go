package db

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

func quarantineDir(dir string) string {
	return filepath.Join(dir, "quarantine")
}

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

func quarantineSSTPath(src, quarantineRoot string, id uint64) error {
	if err := os.MkdirAll(quarantineRoot, 0755); err != nil {
		return err
	}
	dest := filepath.Join(quarantineRoot, fmt.Sprintf("sst_%08d.sst", id))
	if err := os.Rename(src, dest); err != nil {
		return err
	}
	return nil
}

// removeOrphanSSTFiles moves on-disk SSTables not listed in the manifest into
// dir/quarantine/ instead of deleting them, so recoverable inconsistencies are
// not permanently destroyed.
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
	qroot := quarantineDir(db.dir)
	for _, id := range existing {
		if _, ok := live[id]; ok {
			continue
		}
		path := sstFilePath(db.dir, id)
		if err := quarantineSSTPath(path, qroot, id); err != nil {
			log.Printf("pebbledb: quarantine orphan SST %s: %v (leaving in place)", path, err)
		} else {
			log.Printf("pebbledb: quarantined orphan SST %s", path)
		}
	}
}
