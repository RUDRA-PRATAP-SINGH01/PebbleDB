package sstable

import "errors"

var ErrBadMagic = errors.New("sstable: invalid magic number")
var ErrCorruptIndex = errors.New("sstable: corrupt index")
var ErrCorruptFooter = errors.New("sstable: corrupt footer")
var ErrUnsupportedVersion = errors.New("sstable: unsupported version")
var ErrKeyOutOfOrder = errors.New("sstable: keys must be added in ascending order")
var ErrInvalidBlockSize = errors.New("sstable: block size must be positive")
var ErrKeyTooLarge = errors.New("sstable: key or value exceeds maximum size")
var ErrCorruptBloom = errors.New("sstable: corrupt bloom filter")
