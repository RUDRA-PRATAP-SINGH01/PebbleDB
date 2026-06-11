package db

import (
	"log"
	"os"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/sstable"
)

const compactRetryDelay = 100 * time.Millisecond

func (db *DB) compactor() {
	defer close(db.compactDone)
	for range db.compactCh {
		db.compactMu.Lock()

		if err := db.doCompaction(); err != nil {
			db.setBackgroundErr("compaction", err)
			log.Printf("pebbledb: compaction error: %v (retrying)", err)
			time.Sleep(compactRetryDelay)
			select {
			case db.compactCh <- struct{}{}:
			default:
			}
		} else {
			db.clearBackgroundErr()
		}

		db.compactMu.Unlock()
	}
}

func (db *DB) doCompaction() error {
	db.mu.Lock()
	toCompact := db.pickSSTablesForCompactionLocked()
	if len(toCompact) < 2 {
		db.mu.Unlock()
		return nil
	}
	compReaders := make([]*sstable.Reader, len(toCompact))
	copy(compReaders, toCompact)
	if !readersStillPresent(db.sstables, compReaders) {
		db.mu.Unlock()
		return nil
	}
	db.mu.Unlock()

	newReader, _, err := db.mergeSSTables(compReaders)
	if err != nil {
		return err
	}

	oldPaths := make([]string, len(compReaders))
	for i, r := range compReaders {
		oldPaths[i] = r.Path()
	}

	db.mu.Lock()
	if !readersStillPresent(db.sstables, compReaders) {
		db.mu.Unlock()
		newReader.Close()
		os.Remove(newReader.Path())
		return nil
	}

	compactSet := make(map[*sstable.Reader]struct{}, len(compReaders))
	for _, r := range compReaders {
		compactSet[r] = struct{}{}
	}
	newList := make([]*sstable.Reader, 0, len(db.sstables)-len(compReaders)+1)
	for _, r := range db.sstables {
		if _, drop := compactSet[r]; !drop {
			newList = append(newList, r)
		}
	}
	newList = append(newList, newReader)
	db.sstables = newList

	liveIDs, err := liveIDsFromReaders(db.sstables)
	if err != nil {
		db.mu.Unlock()
		newReader.Close()
		os.Remove(newReader.Path())
		return err
	}
	db.mu.Unlock()

	if err := db.manifest.AppendSetFileSet(liveIDs); err != nil {
		newReader.Close()
		os.Remove(newReader.Path())
		return err
	}

	for _, r := range compReaders {
		r.Close()
	}
	for _, p := range oldPaths {
		if p != "" {
			os.Remove(p)
		}
	}

	db.maybeTriggerCompaction()
	return nil
}
