package memtable

import (
	"testing"
)

func TestSkipListPutGet(t *testing.T) {
	sl := NewSkipList()
	sl.Put([]byte("a"), []byte("1"))
	sl.Put([]byte("c"), []byte("3"))
	sl.Put([]byte("b"), []byte("2"))

	val, found, tomb := sl.Get([]byte("b"))
	if !found || tomb || string(val) != "2" {
		t.Errorf("Get(b) = %s, %v, %v; want 2, true, false", val, found, tomb)
	}

	_, found, _ = sl.Get([]byte("d"))
	if found {
		t.Errorf("Get(d) found, want false")
	}
}

func TestSkipListDelete(t *testing.T) {
	sl := NewSkipList()
	sl.Put([]byte("x"), []byte("100"))
	sl.Delete([]byte("x"))
	_, found, tomb := sl.Get([]byte("x"))
	if !found || !tomb {
		t.Errorf("after delete: found=%v, tombstone=%v, want true,true", found, tomb)
	}
}

func TestSkipListIterator(t *testing.T) {
	sl := NewSkipList()
	sl.Put([]byte("b"), []byte("2"))
	sl.Put([]byte("a"), []byte("1"))
	sl.Put([]byte("c"), []byte("3"))

	it := sl.Iterator()
	defer it.Close()

	expected := []string{"a", "b", "c"}
	i := 0
	for it.Valid() {
		if string(it.Key()) != expected[i] {
			t.Errorf("key %d = %s, want %s", i, it.Key(), expected[i])
		}
		i++
		it.Next()
	}
	if i != 3 {
		t.Errorf("iterated %d entries, want 3", i)
	}
}

func TestSkipListSize(t *testing.T) {
	sl := NewSkipList()
	sl.Put([]byte("k1"), []byte("v1"))
	sz1 := sl.Size()
	sl.Put([]byte("k2"), []byte("v2"))
	if sl.Size() <= sz1 {
		t.Errorf("size didn't increase: %d -> %d", sz1, sl.Size())
	}
}

func TestSkipListConcurrent(t *testing.T) {
	sl := NewSkipList()
	done := make(chan bool)
	go func() {
		for i := 0; i < 1000; i++ {
			sl.Put([]byte("key"), []byte("val"))
		}
		done <- true
	}()
	go func() {
		for i := 0; i < 1000; i++ {
			sl.Get([]byte("key"))
		}
		done <- true
	}()
	<-done
	<-done
}

func BenchmarkSkipListPut(b *testing.B) {
	sl := NewSkipList()
	key, val := []byte("key"), []byte("value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sl.Put(key, val)
	}
}