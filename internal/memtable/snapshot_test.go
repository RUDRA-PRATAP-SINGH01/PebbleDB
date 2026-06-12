package memtable

import (
	"sync"
	"testing"
	"time"
)

func TestSnapshotCopyOnRead(t *testing.T) {
	sl := NewSkipList()
	sl.Put([]byte("a"), []byte("1"))

	snap := sl.Snapshot()
	if len(snap) != 1 || string(snap[0].Key) != "a" {
		t.Fatalf("snap = %+v", snap)
	}

	sl.Put([]byte("b"), []byte("2"))
	if len(snap) != 1 {
		t.Fatal("snapshot should not see writes after capture")
	}
}

func TestSnapshotAllowsConcurrentWrites(t *testing.T) {
	sl := NewSkipList()
	for i := 0; i < 100; i++ {
		key := []byte{byte('a' + i%26)}
		sl.Put(key, []byte("v"))
	}

	var wg sync.WaitGroup
	wg.Add(2)

	done := make(chan struct{})
	go func() {
		defer wg.Done()
		snap := sl.Snapshot()
		time.Sleep(50 * time.Millisecond)
		if len(snap) == 0 {
			t.Error("empty snapshot")
		}
		close(done)
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			sl.Put([]byte{byte('A' + i%26)}, []byte("x"))
		}
	}()

	wg.Wait()
	<-done
}
