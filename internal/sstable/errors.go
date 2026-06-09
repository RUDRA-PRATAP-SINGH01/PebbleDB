package sstable

import "errors"

var ErrBadMagic = errors.New("sstable: invalid magic number")
var ErrCorruptIndex = errors.New("sstable: corrupt index")
