package iterator

import (
	"testing"
)

type sliceIter struct {
	keys   [][]byte
	vals   [][]byte
	tombs  []bool
	pri    int
	idx    int
	closed bool
}

func (s *sliceIter) Valid() bool { return !s.closed && s.idx < len(s.keys) }

func (s *sliceIter) Next() error {
	s.idx++
	return nil
}

func (s *sliceIter) Key() []byte   { return s.keys[s.idx] }
func (s *sliceIter) Value() []byte { return s.vals[s.idx] }

func (s *sliceIter) IsTombstone() bool { return s.tombs[s.idx] }

func (s *sliceIter) Seek(key []byte) error {
	s.idx = 0
	for s.idx < len(s.keys) {
		if string(s.keys[s.idx]) >= string(key) {
			break
		}
		s.idx++
	}
	return nil
}

func (s *sliceIter) Close() error {
	s.closed = true
	return nil
}

func TestMergeIteratorNewestWins(t *testing.T) {
	old := &sliceIter{
		keys:  [][]byte{[]byte("a"), []byte("b")},
		vals:  [][]byte{[]byte("1"), []byte("2")},
		tombs: []bool{false, false},
	}
	newer := &sliceIter{
		keys:  [][]byte{[]byte("a"), []byte("c")},
		vals:  [][]byte{[]byte("9"), []byte("3")},
		tombs: []bool{false, false},
	}

	m, err := NewMergeIterator([]Iterator{old, newer}, []int{0, 1})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	if err := m.Seek(nil); err != nil {
		t.Fatal(err)
	}

	var keys []string
	for m.Valid() {
		keys = append(keys, string(m.Key()))
		if err := m.Next(); err != nil {
			t.Fatal(err)
		}
	}
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Fatalf("got keys %v, want [a b c]", keys)
	}
}

func TestMergeIteratorSkipsTombstone(t *testing.T) {
	sst := &sliceIter{
		keys:  [][]byte{[]byte("gone")},
		vals:  [][]byte{nil},
		tombs: []bool{true},
	}
	active := &sliceIter{
		keys:  [][]byte{[]byte("gone")},
		vals:  [][]byte{nil},
		tombs: []bool{true},
	}

	m, err := NewMergeIterator([]Iterator{sst, active}, []int{0, 1})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	if err := m.Seek(nil); err != nil {
		t.Fatal(err)
	}
	if m.Valid() {
		t.Fatal("tombstoned key should not be returned")
	}
}
