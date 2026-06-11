package manifest

import (
	"encoding/binary"
	"hash/crc32"
	"io"
)

const (
	tagNewFile    byte = 0x01
	tagDeleteFile byte = 0x02
	tagSetFileSet byte = 0x03
)

func encodeRecord(payload []byte) []byte {
	checksum := crc32.ChecksumIEEE(payload)
	recordLen := uint32(len(payload) + 4)
	buf := make([]byte, 4+4+len(payload))
	binary.BigEndian.PutUint32(buf[0:4], recordLen)
	binary.BigEndian.PutUint32(buf[4:8], checksum)
	copy(buf[8:], payload)
	return buf
}

func encodeNewFile(sstID uint64) []byte {
	payload := make([]byte, 1+8)
	payload[0] = tagNewFile
	binary.BigEndian.PutUint64(payload[1:9], sstID)
	return encodeRecord(payload)
}

func encodeSetFileSet(ids []uint64) []byte {
	payload := make([]byte, 1+4+8*len(ids))
	payload[0] = tagSetFileSet
	binary.BigEndian.PutUint32(payload[1:5], uint32(len(ids)))
	for i, id := range ids {
		binary.BigEndian.PutUint64(payload[5+i*8:5+(i+1)*8], id)
	}
	return encodeRecord(payload)
}

func decodeRecord(data []byte) (payload []byte, err error) {
	if len(data) < 8 {
		return nil, io.ErrUnexpectedEOF
	}
	recordLen := binary.BigEndian.Uint32(data[0:4])
	if recordLen < 4 {
		return nil, io.ErrUnexpectedEOF
	}
	total := int(4 + recordLen)
	if len(data) < total {
		return nil, io.ErrUnexpectedEOF
	}
	checksum := binary.BigEndian.Uint32(data[4:8])
	payload = data[8:total]
	if crc32.ChecksumIEEE(payload) != checksum {
		return nil, io.ErrUnexpectedEOF
	}
	return payload, nil
}

func applyEdit(liveSet map[uint64]struct{}, payload []byte) error {
	if len(payload) < 1 {
		return io.ErrUnexpectedEOF
	}
	switch payload[0] {
	case tagNewFile:
		if len(payload) < 9 {
			return io.ErrUnexpectedEOF
		}
		id := binary.BigEndian.Uint64(payload[1:9])
		liveSet[id] = struct{}{}
	case tagDeleteFile:
		if len(payload) < 9 {
			return io.ErrUnexpectedEOF
		}
		id := binary.BigEndian.Uint64(payload[1:9])
		delete(liveSet, id)
	case tagSetFileSet:
		if len(payload) < 5 {
			return io.ErrUnexpectedEOF
		}
		count := binary.BigEndian.Uint32(payload[1:5])
		need := 5 + int(count)*8
		if len(payload) < need {
			return io.ErrUnexpectedEOF
		}
		next := make(map[uint64]struct{}, count)
		for i := uint32(0); i < count; i++ {
			id := binary.BigEndian.Uint64(payload[5+i*8 : 5+(i+1)*8])
			next[id] = struct{}{}
		}
		clear(liveSet)
		for id := range next {
			liveSet[id] = struct{}{}
		}
	default:
		return io.ErrUnexpectedEOF
	}
	return nil
}
