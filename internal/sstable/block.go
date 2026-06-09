package sstable

import (
	"encoding/binary"
)

// Block stores a sequence of sorted key-value pairs.
// Format: [keyLen][key][valueLen][value][tombstone] repeated.
type Block struct {
	data []byte
}

// NewBlock creates an empty block.
func NewBlock() *Block {
	return &Block{data: make([]byte, 0)}
}

// Append adds a key-value pair to the block.
func (b *Block) Append(key, value []byte, tombstone bool) {
	keyLen := uint32(len(key))
	valLen := uint32(len(value))
	tombByte := byte(0)
	if tombstone {
		tombByte = 1
	}
	buf := make([]byte, 0, 4+keyLen+4+valLen+1)
	buf = binary.BigEndian.AppendUint32(buf, keyLen)
	buf = append(buf, key...)
	buf = binary.BigEndian.AppendUint32(buf, valLen)
	buf = append(buf, value...)
	buf = append(buf, tombByte)
	b.data = append(b.data, buf...)
}

// Size returns the current size of the block in bytes.
func (b *Block) Size() int {
	return len(b.data)
}

// Bytes returns the raw block data.
func (b *Block) Bytes() []byte {
	return b.data
}

// Reset clears the block for reuse.
func (b *Block) Reset() {
	b.data = b.data[:0]
}

// BlockIterator iterates over entries inside a block.
type BlockIterator struct {
	data      []byte
	pos       int
	key       []byte
	val       []byte
	tombstone bool
}

// NewBlockIterator creates an iterator from raw block bytes.
func NewBlockIterator(data []byte) *BlockIterator {
	return &BlockIterator{data: data, pos: 0}
}

// Next advances to the next entry. Returns false at EOF or on corrupt data.
func (it *BlockIterator) Next() bool {
	if it.pos+4 > len(it.data) {
		return false
	}
	keyLen := binary.BigEndian.Uint32(it.data[it.pos:])
	it.pos += 4
	if it.pos+int(keyLen) > len(it.data) {
		return false
	}
	it.key = it.data[it.pos : it.pos+int(keyLen)]
	it.pos += int(keyLen)
	if it.pos+4 > len(it.data) {
		return false
	}
	valLen := binary.BigEndian.Uint32(it.data[it.pos:])
	it.pos += 4
	if it.pos+int(valLen) > len(it.data) {
		return false
	}
	it.val = it.data[it.pos : it.pos+int(valLen)]
	it.pos += int(valLen)
	if it.pos >= len(it.data) {
		return false
	}
	tombByte := it.data[it.pos]
	it.pos += 1
	it.tombstone = tombByte == 1
	return true
}

func (it *BlockIterator) Key() []byte { return it.key }
func (it *BlockIterator) Value() []byte { return it.val }
func (it *BlockIterator) IsTombstone() bool { return it.tombstone }
