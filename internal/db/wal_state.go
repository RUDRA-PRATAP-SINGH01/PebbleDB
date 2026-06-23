package db

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

const walFlushStateName = "wal.flush"

// walFlushState is written after a successful SST flush is committed to the
// manifest but before WAL truncation. If present on open, WAL bytes before
// FreezeOffset are already reflected in the flushed SST.
type walFlushState struct {
	FreezeOffset int64
	SSTID        uint64
}

func walFlushStatePath(dir string) string {
	return filepath.Join(dir, walFlushStateName)
}

func writeWalFlushState(dir string, st walFlushState) error {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint64(buf[0:8], uint64(st.FreezeOffset))
	binary.BigEndian.PutUint64(buf[8:16], st.SSTID)
	tmp := walFlushStatePath(dir) + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, walFlushStatePath(dir))
}

func readWalFlushState(dir string) (walFlushState, bool, error) {
	data, err := os.ReadFile(walFlushStatePath(dir))
	if err != nil {
		if os.IsNotExist(err) {
			return walFlushState{}, false, nil
		}
		return walFlushState{}, false, err
	}
	if len(data) < 16 {
		_ = os.Remove(walFlushStatePath(dir))
		return walFlushState{}, false, fmt.Errorf("%w: got %d bytes, want 16", ErrCorruptWalFlushState, len(data))
	}
	return walFlushState{
		FreezeOffset: int64(binary.BigEndian.Uint64(data[0:8])),
		SSTID:        binary.BigEndian.Uint64(data[8:16]),
	}, true, nil
}

func removeWalFlushState(dir string) error {
	err := os.Remove(walFlushStatePath(dir))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// walReplayStartOffset returns the byte offset to begin WAL replay from.
// If the WAL was truncated below FreezeOffset (crash after truncate), replay from 0.
func (db *DB) walReplayStartOffset() (int64, error) {
	st, ok, err := readWalFlushState(db.dir)
	if err != nil || !ok {
		return 0, err
	}
	if !db.manifest.Contains(st.SSTID) {
		return 0, nil
	}
	if st.FreezeOffset < 0 {
		return 0, nil
	}
	walPath := filepath.Join(db.dir, "wal.log")
	fi, err := os.Stat(walPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if fi.Size() < st.FreezeOffset {
		return 0, nil
	}
	return st.FreezeOffset, nil
}
