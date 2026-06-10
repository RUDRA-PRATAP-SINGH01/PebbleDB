package sstable

import (
	"encoding/binary"
	"io"
)

const (
	// footerSize is 48 bytes; bytes 40-47 are reserved for future use (zero padded).
	footerSize     = 48
	magicNumber    = 0x88e241b3
	currentVersion = 2
)

// Footer is written at the end of the SSTable.
type Footer struct {
	IndexOffset uint64
	IndexLength uint64
	BloomOffset uint64
	BloomLength uint64
	Version     uint32
	Magic       uint32
}

// Encode writes the footer to a byte slice.
func (f *Footer) Encode() []byte {
	buf := make([]byte, footerSize)
	binary.BigEndian.PutUint64(buf[0:8], f.IndexOffset)
	binary.BigEndian.PutUint64(buf[8:16], f.IndexLength)
	binary.BigEndian.PutUint64(buf[16:24], f.BloomOffset)
	binary.BigEndian.PutUint64(buf[24:32], f.BloomLength)
	binary.BigEndian.PutUint32(buf[32:36], f.Version)
	binary.BigEndian.PutUint32(buf[36:40], f.Magic)
	return buf
}

// Decode reads the footer from a byte slice.
func (f *Footer) Decode(data []byte) error {
	if len(data) < footerSize {
		return ErrCorruptFooter
	}
	f.IndexOffset = binary.BigEndian.Uint64(data[0:8])
	f.IndexLength = binary.BigEndian.Uint64(data[8:16])
	f.BloomOffset = binary.BigEndian.Uint64(data[16:24])
	f.BloomLength = binary.BigEndian.Uint64(data[24:32])
	f.Version = binary.BigEndian.Uint32(data[32:36])
	f.Magic = binary.BigEndian.Uint32(data[36:40])
	return nil
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
	if err := footer.Decode(buf); err != nil {
		return nil, err
	}
	if footer.Magic != magicNumber {
		return nil, ErrBadMagic
	}
	if footer.Version != currentVersion {
		return nil, ErrUnsupportedVersion
	}
	return footer, nil
}
