//go:build windows

package db

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	modKernel32        = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx     = modKernel32.NewProc("LockFileEx")
	procUnlockFileEx   = modKernel32.NewProc("UnlockFileEx")
	lockFileOverlapped syscall.Overlapped
)

const (
	lockfileExclusiveLock   = 0x2
	lockfileFailImmediately = 0x1
)

func lockFile(f *os.File) error {
	h := syscall.Handle(f.Fd())
	ret, _, err := procLockFileEx.Call(
		uintptr(h),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0,
		^uintptr(0),
		^uintptr(0),
		uintptr(unsafe.Pointer(&lockFileOverlapped)),
	)
	if ret == 0 {
		if errno, ok := err.(syscall.Errno); ok && errno == 33 { // ERROR_LOCK_VIOLATION
			return ErrDatabaseLocked
		}
		return err
	}
	return nil
}

func unlockFile(f *os.File) error {
	h := syscall.Handle(f.Fd())
	ret, _, err := procUnlockFileEx.Call(
		uintptr(h),
		0,
		^uintptr(0),
		^uintptr(0),
		uintptr(unsafe.Pointer(&lockFileOverlapped)),
	)
	if ret == 0 {
		return err
	}
	return nil
}
