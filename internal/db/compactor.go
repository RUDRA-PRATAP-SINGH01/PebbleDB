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
		for {
			if err := db.doCompaction(); err != nil {
				db.setBackgroundErr("compaction", err)
				log.Printf("pebbledb: compaction error: %v (retrying)", err)
				time.Sleep(compactRetryDelay)
				break
			}
			db.clearBackgroundErrOp("compaction")

			db.mu.RLock()
			more := len(db.sstables) >= db.compactThreshold
			db.mu.RUnlock()
			if !more {
				break
			}
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
	oldLiveIDs, err := liveIDsFromReaders(db.sstables)
	if err != nil {
		db.mu.Unlock()
		return err
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
	maybeCrash(CrashAfterMergeClose)

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

	liveIDs, err := liveIDsFromReaders(newList)
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
	maybeCrash(CrashAfterManifestSetFileSet)
	if err := db.manifest.MaybeCompact(); err != nil {
		return err
	}

	oldPaths := make([]string, len(compReaders))
	for i, r := range compReaders {
		oldPaths[i] = r.Path()
	}

	db.mu.Lock()
	if !readersStillPresent(db.sstables, compReaders) {
		db.mu.Unlock()
		if rbErr := db.manifest.AppendSetFileSet(oldLiveIDs); rbErr != nil {
			log.Printf("pebbledb: compaction manifest rollback failed: %v", rbErr)
		}
		newReader.Close()
		os.Remove(newReader.Path())
		return nil
	}

	db.sstables = newList
	db.mu.Unlock()
	maybeCrash(CrashAfterSSTablesUpdate)

	db.trackReader(newReader)

	for _, r := range compReaders {
		if err := r.Discard(); err != nil {
			log.Printf("pebbledb: discard compacted SST: %v", err)
		}
	}
	for _, p := range oldPaths {
		if p != "" {
			if err := removeSSTPath(p); err != nil {
				log.Printf("pebbledb: remove compacted SST %s: %v", p, err)
			}
		}
	}
	maybeCrash(CrashAfterDeleteOldSSTs)

	return nil
}
