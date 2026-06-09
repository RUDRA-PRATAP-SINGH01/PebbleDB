package sstable

import (
	"encoding/binary"
)

// IndexEntry points to a block.
type IndexEntry struct {
	LastKey []byte
	Offset  uint64
	Length  uint64
}

// IndexBlock holds all index entries.
type IndexBlock struct {
	entries []IndexEntry
}

func (idx *IndexBlock) Add(entry IndexEntry) {
	idx.entries = append(idx.entries, entry)
}

// Encode serialises the index block to bytes.
func (idx *IndexBlock) Encode() []byte {
	buf := make([]byte, 0)
	for _, e := range idx.entries {
		// key length + key + offset + length
		keyLen := uint32(len(e.LastKey))
		buf = binary.BigEndian.AppendUint32(buf, keyLen)
		buf = append(buf, e.LastKey...)
		buf = binary.BigEndian.AppendUint64(buf, e.Offset)
		buf = binary.BigEndian.AppendUint64(buf, e.Length)
	}
	return buf
}