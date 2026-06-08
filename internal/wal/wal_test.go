package wal

import (
	"os"
	"testing"
)

func TestWALAppendAndReplay(t *testing.T) {
	path := "test_wal.log"
	defer os.Remove(path)

	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	recs := []Record{
		{Key: []byte("k1"), Value: []byte("v1"), Tombstone: false},
		{Key: []byte("k2"), Value: []byte("v2"), Tombstone: false},
		{Key: []byte("k3"), Value: nil, Tombstone: true},
	}
	for _, r := range recs {
		if err := w.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	w.Sync()

	// Replay
	var replayed []Record
	err = Replay(path, func(r Record) error {
		replayed = append(replayed, r)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed) != len(recs) {
		t.Fatalf("got %d records, want %d", len(replayed), len(recs))
	}
	for i := range recs {
		if string(replayed[i].Key) != string(recs[i].Key) {
			t.Errorf("key mismatch")
		}
	}
}

func TestWALTruncate(t *testing.T) {
	path := "test_wal_truncate.log"
	defer os.Remove(path)

	w, _ := Open(path)
	w.Append(Record{Key: []byte("x"), Value: []byte("y")})
	w.Truncate()

	// Replay should be empty
	count := 0
	Replay(path, func(r Record) error {
		count++
		return nil
	})
	if count != 0 {
		t.Errorf("truncated WAL still has %d entries", count)
	}
}