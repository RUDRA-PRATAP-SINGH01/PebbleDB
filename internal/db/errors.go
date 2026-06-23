package db

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("key not found")
var ErrClosed = errors.New("database closed")
var ErrCloseFlushTimeout = errors.New("close: timed out waiting for flush to complete")
var ErrCloseWorkerTimeout = errors.New("close: timed out waiting for background workers to stop")

// BackgroundError is returned when a background operation failed.
type BackgroundError struct {
	Op  string
	Err error
}

func (e *BackgroundError) Error() string {
	return fmt.Sprintf("db: background %s failed: %v", e.Op, e.Err)
}

func (e *BackgroundError) Unwrap() error {
	return e.Err
}
