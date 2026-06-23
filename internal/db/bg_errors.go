package db

import (
	"errors"
	"sort"
	"sync"
)

// writeBlockingBackgroundOps are background failure kinds that block new writes.
// Reads continue from memtables and SSTables so durable data stays available.
var writeBlockingBackgroundOps = map[string]struct{}{
	"wal":   {},
	"flush": {},
}

type backgroundErrStore struct {
	mu   sync.RWMutex
	byOp map[string]error
}

func newBackgroundErrStore() *backgroundErrStore {
	return &backgroundErrStore{byOp: make(map[string]error)}
}

func (db *DB) setBackgroundErr(op string, err error) {
	if err == nil {
		return
	}
	db.bgErrs.mu.Lock()
	db.bgErrs.byOp[op] = err
	db.bgErrs.mu.Unlock()
}

func (db *DB) clearBackgroundErrOp(op string) {
	db.bgErrs.mu.Lock()
	delete(db.bgErrs.byOp, op)
	db.bgErrs.mu.Unlock()
}

func (db *DB) backgroundErr() error {
	return db.bgErrs.join(nil)
}

func (db *DB) writeBlockingBackgroundErr() error {
	if !db.blockWritesOnFlushError {
		return db.bgErrs.join(map[string]struct{}{"wal": {}})
	}
	return db.bgErrs.join(writeBlockingBackgroundOps)
}

func (s *backgroundErrStore) join(only map[string]struct{}) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.byOp) == 0 {
		return nil
	}

	ops := make([]string, 0, len(s.byOp))
	for op := range s.byOp {
		if only != nil {
			if _, ok := only[op]; !ok {
				continue
			}
		}
		ops = append(ops, op)
	}
	if len(ops) == 0 {
		return nil
	}
	sort.Strings(ops)

	var joined error
	for _, op := range ops {
		joined = errors.Join(joined, &BackgroundError{Op: op, Err: s.byOp[op]})
	}
	return joined
}
