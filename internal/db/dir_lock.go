package db

import (
	"os"
	"path/filepath"
)

const lockFileName = "LOCK"

func acquireDirLock(dir string) (*os.File, error) {
	path := filepath.Join(dir, lockFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		if err == ErrDatabaseLocked {
			return nil, err
		}
		return nil, err
	}
	return f, nil
}

func releaseDirLock(f *os.File) {
	if f == nil {
		return
	}
	_ = unlockFile(f)
	_ = f.Close()
}
