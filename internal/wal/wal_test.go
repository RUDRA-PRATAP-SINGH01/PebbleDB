package wal

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestWALAppendAndReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.log")

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
	if err := w.Sync(); err != nil {
		t.Fatal(err)
	}

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
			t.Errorf("key mismatch at %d: got %q, want %q", i, replayed[i].Key, recs[i].Key)
		}
	}
}

func TestWALTruncate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.log")

	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := w.Append(Record{Key: []byte("x"), Value: []byte("y")}); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := w.Truncate(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	count := 0
	err = Replay(path, func(r Record) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("truncated WAL still has %d entries, want 0", count)
	}
}

func TestWALTruncateBefore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.log")

	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := w.Append(Record{Key: []byte("old1"), Value: []byte("a")}); err != nil {
		t.Fatal(err)
	}
	if err := w.Append(Record{Key: []byte("old2"), Value: []byte("b")}); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync(); err != nil {
		t.Fatal(err)
	}

	cutoff, err := w.Size()
	if err != nil {
		t.Fatal(err)
	}

	if err := w.Append(Record{Key: []byte("new"), Value: []byte("c")}); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := w.TruncateBefore(cutoff); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	var keys []string
	err = Replay(path, func(r Record) error {
		keys = append(keys, string(r.Key))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != "new" {
		t.Errorf("got keys %v, want [new]", keys)
	}
}

func TestReplayRejectsOversizedKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := binary.Write(f, binary.BigEndian, uint32(1024)); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	limits := ReplayLimits{MaxFileSize: 4096, MaxKeySize: 16, MaxValueSize: 16, MaxRecordSize: 64}
	err = ReplayWithLimits(path, limits, func(Record) error { return nil })
	if err != ErrKeyTooLarge {
		t.Fatalf("got %v, want ErrKeyTooLarge", err)
	}
}

func TestAppendRejectsOversizedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.log")
	limits := ReplayLimits{MaxFileSize: 4096, MaxKeySize: 8, MaxValueSize: 8, MaxRecordSize: 32}
	w, err := OpenWithLimits(path, limits)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	err = w.Append(Record{Key: []byte("123456789"), Value: []byte("x")})
	if err != ErrKeyTooLarge {
		t.Fatalf("got %v, want ErrKeyTooLarge", err)
	}
}
