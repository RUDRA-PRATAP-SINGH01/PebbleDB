package wal

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestWALAppendBatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.log")

	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	recs := []Record{
		{Key: []byte("a"), Value: []byte("1")},
		{Key: []byte("b"), Value: []byte("2")},
		{Key: []byte("c"), Value: nil, Tombstone: true},
	}
	if err := w.AppendBatch(recs); err != nil {
		t.Fatal(err)
	}

	var replayed []Record
	if err := Replay(path, func(r Record) error {
		replayed = append(replayed, r)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(replayed) != len(recs) {
		t.Fatalf("got %d records, want %d", len(replayed), len(recs))
	}
}

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

func TestWALOffset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.log")

	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	off, err := w.Offset()
	if err != nil {
		t.Fatal(err)
	}
	if off != 0 {
		t.Fatalf("empty WAL offset = %d, want 0", off)
	}

	if err := w.Append(Record{Key: []byte("k"), Value: []byte("v")}); err != nil {
		t.Fatal(err)
	}
	off, err = w.Offset()
	if err != nil {
		t.Fatal(err)
	}
	size, err := w.Size()
	if err != nil {
		t.Fatal(err)
	}
	if int64(off) != size {
		t.Fatalf("Offset()=%d Size()=%d, want equal", off, size)
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

func TestReplaySalvagesTrailingPartialRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.log")

	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(Record{Key: []byte("ok"), Value: []byte("v")}); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0, 0, 0, 5}); err != nil { // partial key length only
		t.Fatal(err)
	}
	f.Close()

	count := 0
	_, err = ReplayFromWithRecovery(path, DefaultReplayLimits(), 0, func(Record) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("replay with recovery: %v", err)
	}
	if count != 1 {
		t.Fatalf("replayed %d records, want 1", count)
	}
}

func TestReplayFromOffsetSkipsPrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wal.log")

	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(Record{Key: []byte("old"), Value: []byte("1")}); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync(); err != nil {
		t.Fatal(err)
	}
	cutoff, err := w.Size()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(Record{Key: []byte("new"), Value: []byte("2")}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	var keys []string
	_, err = ReplayFromWithRecovery(path, DefaultReplayLimits(), cutoff, func(r Record) error {
		keys = append(keys, string(r.Key))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != "new" {
		t.Fatalf("got %v, want [new]", keys)
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
