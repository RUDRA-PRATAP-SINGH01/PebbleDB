package sstable

import (
	"bytes"
	"os"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/bloom"
)

const defaultBloomExpectedEntries = 1024

type Writer struct {
	file      *os.File
	tmpPath   string
	finalPath string
	blockSize int
	current   *Block
	index     *IndexBlock
	bloom     *bloom.Filter
	lastKey   []byte
	offset    uint64
}

func NewWriter(path string, blockSize int) (*Writer, error) {
	if blockSize <= 0 {
		return nil, ErrInvalidBlockSize
	}
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return nil, err
	}
	return &Writer{
		file:      f,
		tmpPath:   tmpPath,
		finalPath: path,
		blockSize: blockSize,
		current:   NewBlock(),
		index:     &IndexBlock{},
		bloom:     bloom.New(defaultBloomExpectedEntries, 0.01),
	}, nil
}

func (w *Writer) cleanup() {
	if w.file != nil {
		w.file.Close()
		w.file = nil
	}
	if w.tmpPath != "" {
		os.Remove(w.tmpPath)
		w.tmpPath = ""
	}
}

func (w *Writer) Add(key, value []byte, tombstone bool) error {
	if w.lastKey != nil && bytes.Compare(key, w.lastKey) <= 0 {
		return ErrKeyOutOfOrder
	}
	entrySize := 4 + len(key) + 4 + len(value) + 1
	if w.current.Size()+entrySize > w.blockSize && w.current.Size() > 0 {
		if err := w.flushBlock(); err != nil {
			return err
		}
	}
	if err := w.current.Append(key, value, tombstone); err != nil {
		return err
	}
	w.bloom.Add(key)
	w.lastKey = append([]byte(nil), key...)
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
	if w.current.Size() > 0 {
		if err := w.flushBlock(); err != nil {
			w.cleanup()
			return err
		}
	}
	indexData := w.index.Encode()
	indexOffset := w.offset
	if _, err := w.file.Write(indexData); err != nil {
		w.cleanup()
		return err
	}
	w.offset += uint64(len(indexData))

	bloomData := w.bloom.Encode()
	bloomOffset := w.offset
	if _, err := w.file.Write(bloomData); err != nil {
		w.cleanup()
		return err
	}
	w.offset += uint64(len(bloomData))

	footer := Footer{
		IndexOffset: indexOffset,
		IndexLength: uint64(len(indexData)),
		BloomOffset: bloomOffset,
		BloomLength: uint64(len(bloomData)),
		Version:     currentVersion,
		Magic:       magicNumber,
	}
	footerData := footer.Encode()
	if _, err := w.file.Write(footerData); err != nil {
		w.cleanup()
		return err
	}
	if err := w.file.Sync(); err != nil {
		w.cleanup()
		return err
	}
	if err := w.file.Close(); err != nil {
		w.cleanup()
		return err
	}
	w.file = nil
	if err := os.Rename(w.tmpPath, w.finalPath); err != nil {
		os.Remove(w.tmpPath)
		w.tmpPath = ""
		return err
	}
	w.tmpPath = ""
	return nil
}
