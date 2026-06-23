//go:build unix

package db

import (
	"os"
	"syscall"
)

func lockFile(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return nil
	}
	if errno, ok := err.(syscall.Errno); ok {
		if errno == syscall.EWOULDBLOCK || errno == syscall.EAGAIN {
			return ErrDatabaseLocked
		}
	}
	return err
}

func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
