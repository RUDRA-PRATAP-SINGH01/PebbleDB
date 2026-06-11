package bloom

import (
	"encoding/binary"
	"hash/fnv"
	"math"
)

// Filter is a Bloom filter.
type Filter struct {
	bits []byte
	k    uint // number of hash functions
	n    uint // number of inserted keys (for stats)
	m    uint // size in bits
}

// New creates a Bloom filter with expected entries and false positive rate.
func New(expectedEntries uint, falsePositiveRate float64) *Filter {
	// m = -n * ln(p) / (ln(2))^2
	m := uint(-float64(expectedEntries) * math.Log(falsePositiveRate) / (math.Ln2 * math.Ln2))
	// k = (m / n) * ln(2)
	k := uint(math.Ceil(float64(m) / float64(expectedEntries) * math.Ln2))
	if k < 1 {
		k = 1
	}
	if k > 30 {
		k = 30
	}
	// align m to byte boundary
	m = (m + 7) & ^uint(7)
	return &Filter{
		bits: make([]byte, m/8),
		k:    k,
		n:    0,
		m:    m,
	}
}

// Add inserts a key into the Bloom filter.
func (f *Filter) Add(key []byte) {
	if f == nil || f.m == 0 {
		return
	}
	h := fnv.New64a()
	h.Write(key)
	sum := h.Sum64()
	h1 := sum
	h2 := sum >> 32
	for i := uint(0); i < f.k; i++ {
		idx := (h1 + uint64(i)*h2) % uint64(f.m)
		f.bits[idx/8] |= 1 << (idx % 8)
	}
	f.n++
}

// MayContain returns true if the key might be in the set (false positive possible).
func (f *Filter) MayContain(key []byte) bool {
	if f == nil || f.m == 0 {
		return true
	}
	h := fnv.New64a()
	h.Write(key)
	sum := h.Sum64()
	h1 := sum
	h2 := sum >> 32
	for i := uint(0); i < f.k; i++ {
		idx := (h1 + uint64(i)*h2) % uint64(f.m)
		if f.bits[idx/8]&(1<<(idx%8)) == 0 {
			return false
		}
	}
	return true
}

// Encode serializes the Bloom filter to bytes (k, m, bits).
func (f *Filter) Encode() []byte {
	buf := make([]byte, 8+len(f.bits))
	binary.BigEndian.PutUint32(buf[0:4], uint32(f.k))
	binary.BigEndian.PutUint32(buf[4:8], uint32(f.m))
	copy(buf[8:], f.bits)
	return buf
}

// Decode restores a Bloom filter from bytes.
func Decode(data []byte) *Filter {
	if len(data) < 8 {
		return nil
	}
	k := uint(binary.BigEndian.Uint32(data[0:4]))
	m := uint(binary.BigEndian.Uint32(data[4:8]))
	if m == 0 || k == 0 {
		return nil
	}
	bits := data[8:]
	expectedLen := (m + 7) / 8
	if uint(len(bits)) < expectedLen {
		return nil
	}
	bitsCopy := make([]byte, expectedLen)
	copy(bitsCopy, bits[:expectedLen])
	return &Filter{
		bits: bitsCopy,
		k:    k,
		m:    m,
		n:    0,
	}
}
