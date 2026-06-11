package memtable

import "testing"

func TestSkipListIteratorSeek(t *testing.T) {
	sl := NewSkipList()
	sl.Put([]byte("a"), []byte("1"))
	sl.Put([]byte("c"), []byte("3"))
	sl.Put([]byte("e"), []byte("5"))

	it := sl.Iterator()
	defer it.Close()

	it.Seek([]byte("b"))
	if !it.Valid() || string(it.Key()) != "c" {
		t.Fatalf("seek b: key=%q valid=%v", it.Key(), it.Valid())
	}

	it.Seek([]byte("c"))
	if !it.Valid() || string(it.Key()) != "c" {
		t.Fatalf("seek c: key=%q", it.Key())
	}

	it.Seek([]byte("z"))
	if it.Valid() {
		t.Fatal("seek past end should be invalid")
	}
}
