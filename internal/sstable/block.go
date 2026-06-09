package sstable

import (
	"encoding/binary"
)

// Block stores a sequence of sorted key-value pairs.
// Format: [keyLen][key][valueLen][value] repeated.
type Block struct {
	data []byte
}

// NewBlock creates an empty block.
func NewBlock() *Block {
	return &Block{data: make([]byte, 0)}
}

// Append adds a key-value pair to the block.
func (b *Block) Append(key, value []byte) {
	keyLen := uint32(len(key))
	valLen := uint32(len(value))
	buf := make([]byte, 0, 4+keyLen+4+valLen)
	buf = binary.BigEndian.AppendUint32(buf, keyLen)
	buf = append(buf, key...)
	buf = binary.BigEndian.AppendUint32(buf, valLen)
	buf = append(buf, value...)
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
	data []byte
	pos  int
	key  []byte
	val  []byte
}

// NewBlockIterator creates an iterator from raw block bytes.
func NewBlockIterator(data []byte) *BlockIterator {
	return &BlockIterator{data: data, pos: 0}
}

// Next advances to the next entry. Returns false at EOF.
func (it *BlockIterator) Next() bool {
	if it.pos >= len(it.data) {
		return false
	}
	// read key length
	keyLen := binary.BigEndian.Uint32(it.data[it.pos:])
	it.pos += 4
	// read key
	it.key = it.data[it.pos : it.pos+int(keyLen)]
	it.pos += int(keyLen)
	// read value length
	valLen := binary.BigEndian.Uint32(it.data[it.pos:])
	it.pos += 4
	// read value
	it.val = it.data[it.pos : it.pos+int(valLen)]
	it.pos += int(valLen)
	return true
}

func (it *BlockIterator) Key() []byte   { return it.key }
func (it *BlockIterator) Value() []byte { return it.val }