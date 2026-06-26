package db

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const lockFileName = "LOCK"

func acquireDirLock(dir string) (*os.File, error) {
	path := filepath.Join(dir, lockFileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		if isDirLockContention(err) {
			return nil, ErrDatabaseLocked
		}
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

// isDirLockContention reports whether err means another process holds the LOCK file.
func isDirLockContention(err error) bool {
	if errors.Is(err, syscall.Errno(32)) { // Windows ERROR_SHARING_VIOLATION
		return true
	}
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return true
	}
	var pe *os.PathError
	if errors.As(err, &pe) {
		msg := strings.ToLower(pe.Err.Error())
		if strings.Contains(msg, "used by another process") ||
			strings.Contains(msg, "resource temporarily unavailable") ||
			strings.Contains(msg, "would block") {
			return true
		}
	}
	return false
}

func releaseDirLock(f *os.File) {
	if f == nil {
		return
	}
	_ = unlockFile(f)
	_ = f.Close()
}
