package sstable

import (
	"encoding/binary"
	"io"
)

const (
	footerSize   = 32
	magicNumber  = 0x88e241b3
	version      = 1
)

// Footer is written at the end of the SSTable.
type Footer struct {
	IndexOffset   uint64
	IndexLength   uint64
	Version       uint32
	Magic         uint32
}

// Encode writes the footer to a byte slice.
func (f *Footer) Encode() []byte {
	buf := make([]byte, footerSize)
	binary.BigEndian.PutUint64(buf[0:8], f.IndexOffset)
	binary.BigEndian.PutUint64(buf[8:16], f.IndexLength)
	binary.BigEndian.PutUint32(buf[16:20], f.Version)
	binary.BigEndian.PutUint32(buf[20:24], f.Magic)
	return buf
}

// Decode reads the footer from a byte slice.
func (f *Footer) Decode(data []byte) {
	f.IndexOffset = binary.BigEndian.Uint64(data[0:8])
	f.IndexLength = binary.BigEndian.Uint64(data[8:16])
	f.Version = binary.BigEndian.Uint32(data[16:20])
	f.Magic = binary.BigEndian.Uint32(data[20:24])
}

// ReadFooter reads and validates the footer from the end of the file.
func ReadFooter(r io.ReadSeeker) (*Footer, error) {
	if _, err := r.Seek(-footerSize, io.SeekEnd); err != nil {
		return nil, err
	}
	buf := make([]byte, footerSize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	footer := &Footer{}
	footer.Decode(buf)
	if footer.Magic != magicNumber {
		return nil, ErrBadMagic
	}
	return footer, nil
}