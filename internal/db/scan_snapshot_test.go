package db

import (
	"sync"
	"testing"
	"time"
)

func TestScanDoesNotBlockWrites(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for i := 0; i < 50; i++ {
		key := []byte{byte('a' + i%26)}
		if err := db.Put(key, []byte("v")); err != nil {
			t.Fatal(err)
		}
	}

	it, err := db.Scan(nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	writeDone := make(chan struct{})
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			key := []byte{byte('A' + i%26)}
			if err := db.Put(key, []byte("new")); err != nil {
				t.Errorf("put blocked during scan: %v", err)
				return
			}
		}
		close(writeDone)
	}()

	deadline := time.After(2 * time.Second)
	select {
	case <-writeDone:
	case <-deadline:
		t.Fatal("writes blocked while scan iterator is open")
	}

	for it.Valid() {
		_ = it.Next()
	}
	it.Close()
	wg.Wait()
}
