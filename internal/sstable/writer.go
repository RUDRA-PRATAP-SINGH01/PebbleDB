package sstable

import (
	"os"
)

type Writer struct {
	file       *os.File
	blockSize  int
	current    *Block
	index      *IndexBlock
	lastKey    []byte
	offset     uint64
}

func NewWriter(path string, blockSize int) (*Writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &Writer{
		file:      f,
		blockSize: blockSize,
		current:   NewBlock(),
		index:     &IndexBlock{},
	}, nil
}

func (w *Writer) Add(key, value []byte) error {
	// Estimate size increase: keyLen(4) + key + valueLen(4) + value
	entrySize := 4 + len(key) + 4 + len(value)
	if w.current.Size()+entrySize > w.blockSize && w.current.Size() > 0 {
		if err := w.flushBlock(); err != nil {
			return err
		}
	}
	w.current.Append(key, value)
	w.lastKey = key
	return nil
}

func (w *Writer) flushBlock() error {
	if w.current.Size() == 0 {
		return nil
	}
	blockData := w.current.Bytes()
	n, err := w.file.Write(blockData)
	if err != nil {
		return err
	}
	// Record index entry
	w.index.Add(IndexEntry{
		LastKey: append([]byte(nil), w.lastKey...),
		Offset:  w.offset,
		Length:  uint64(n),
	})
	w.offset += uint64(n)
	w.current.Reset()
	return nil
}

func (w *Writer) Close() error {
	// flush any remaining block
	if w.current.Size() > 0 {
		if err := w.flushBlock(); err != nil {
			return err
		}
	}
	// write index block
	indexData := w.index.Encode()
	indexOffset := w.offset
	if _, err := w.file.Write(indexData); err != nil {
		return err
	}
	w.offset += uint64(len(indexData))

	// write footer
	footer := Footer{
		IndexOffset: indexOffset,
		IndexLength: uint64(len(indexData)),
		Version:     version,
		Magic:       magicNumber,
	}
	footerData := footer.Encode()
	if _, err := w.file.Write(footerData); err != nil {
		return err
	}
	return w.file.Close()
}