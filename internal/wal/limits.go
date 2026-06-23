package wal

import "errors"

const (
	DefaultMaxWALFileSize int64  = 64 << 20 // 64 MiB
	DefaultMaxKeySize     uint32 = 1 << 20  // 1 MiB
	DefaultMaxValueSize   uint32 = 16 << 20 // 16 MiB
	DefaultMaxRecordSize  uint32 = 17 << 20 // key + value + overhead
	recordHeaderSize             = 4 + 4 + 1 + 4
)

var (
	ErrWALTooLarge    = errors.New("wal: file exceeds maximum size")
	ErrKeyTooLarge    = errors.New("wal: key exceeds maximum size")
	ErrValueTooLarge  = errors.New("wal: value exceeds maximum size")
	ErrRecordTooLarge = errors.New("wal: record exceeds maximum size")
)

// ReplayLimits bounds WAL replay and append sizes to prevent OOM on corrupt data.
type ReplayLimits struct {
	MaxFileSize   int64
	MaxKeySize    uint32
	MaxValueSize  uint32
	MaxRecordSize uint32
}

// DefaultReplayLimits returns safe default size limits.
func DefaultReplayLimits() ReplayLimits {
	return ReplayLimits{
		MaxFileSize:   DefaultMaxWALFileSize,
		MaxKeySize:    DefaultMaxKeySize,
		MaxValueSize:  DefaultMaxValueSize,
		MaxRecordSize: DefaultMaxRecordSize,
	}
}

// WithDefaults fills zero fields with package defaults.
func (l ReplayLimits) WithDefaults() ReplayLimits {
	d := DefaultReplayLimits()
	if l.MaxFileSize <= 0 {
		l.MaxFileSize = d.MaxFileSize
	}
	if l.MaxKeySize == 0 {
		l.MaxKeySize = d.MaxKeySize
	}
	if l.MaxValueSize == 0 {
		l.MaxValueSize = d.MaxValueSize
	}
	if l.MaxRecordSize == 0 {
		l.MaxRecordSize = d.MaxRecordSize
	}
	return l
}

func (l ReplayLimits) validateRecord(keyLen, valueLen uint32) error {
	if keyLen > l.MaxKeySize {
		return ErrKeyTooLarge
	}
	if valueLen > l.MaxValueSize {
		return ErrValueTooLarge
	}
	recSize := uint64(recordHeaderSize) + uint64(keyLen) + uint64(valueLen)
	if recSize > uint64(l.MaxRecordSize) {
		return ErrRecordTooLarge
	}
	return nil
}
