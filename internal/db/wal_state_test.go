package db

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/manifest"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/sstable"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"
)

func TestWalFlushStateCorruptFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(walFlushStatePath(dir), []byte("short"), 0644); err != nil {
		t.Fatal(err)
	}
	_, ok, err := readWalFlushState(dir)
	if err == nil {
		t.Fatalf("expected corrupt wal.flush error, ok=%v", ok)
	}
	if !errors.Is(err, ErrCorruptWalFlushState) {
		t.Fatalf("err = %v, want ErrCorruptWalFlushState", err)
	}
	if _, statErr := os.Stat(walFlushStatePath(dir)); statErr == nil {
		t.Fatal("corrupt wal.flush should be removed")
	}
}

func TestWalReplayStartOffsetUsesFlushState(t *testing.T) {
	dir := t.TempDir()
	m, err := manifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.AppendNewFile(7); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writeWalFlushState(dir, walFlushState{FreezeOffset: 128, SSTID: 7}); err != nil {
		t.Fatal(err)
	}
	walPath := filepath.Join(dir, "wal.log")
	if err := os.WriteFile(walPath, make([]byte, 200), 0644); err != nil {
		t.Fatal(err)
	}

	m2, err := manifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()

	db := &DB{dir: dir, manifest: m2}
	off, err := db.walReplayStartOffset()
	if err != nil {
		t.Fatal(err)
	}
	if off != 128 {
		t.Fatalf("replay offset = %d, want 128", off)
	}
}

func TestWalReplayStartOffsetWhenWalTruncatedBelowFreeze(t *testing.T) {
	dir := t.TempDir()
	m, err := manifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.AppendNewFile(3); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	walPath := filepath.Join(dir, "wal.log")
	w, err := wal.OpenWithLimits(walPath, wal.DefaultReplayLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(wal.Record{Key: []byte("tail"), Value: []byte("only")}); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	if err := writeWalFlushState(dir, walFlushState{FreezeOffset: 4096, SSTID: 3}); err != nil {
		t.Fatal(err)
	}

	m2, err := manifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()

	db := &DB{dir: dir, manifest: m2}
	off, err := db.walReplayStartOffset()
	if err != nil {
		t.Fatal(err)
	}
	if off != 0 {
		t.Fatalf("replay offset = %d, want 0 when wal smaller than freeze offset", off)
	}
}

func TestGetAllowedDuringBackgroundError(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Put([]byte("k"), []byte("v")); err != nil {
		t.Fatal(err)
	}
	db.setBackgroundErr("compaction", os.ErrPermission)

	val, err := db.Get([]byte("k"))
	if err != nil {
		t.Fatalf("Get during background error: %v", err)
	}
	if string(val) != "v" {
		t.Fatalf("got %q, want v", val)
	}
}

func TestScanAllowedDuringBackgroundError(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Put([]byte("k"), []byte("v")); err != nil {
		t.Fatal(err)
	}
	db.setBackgroundErr("flush", os.ErrPermission)

	it, err := db.Scan(nil, nil)
	if err != nil {
		t.Fatalf("Scan during background error: %v", err)
	}
	defer it.Close()
	if !it.Valid() {
		t.Fatal("expected scan iterator")
	}
}

func TestPutBlockedDuringWalBackgroundError(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.setBackgroundErr("wal", os.ErrPermission)
	err = db.Put([]byte("k"), []byte("v"))
	if err == nil {
		t.Fatal("expected Put to fail during WAL background error")
	}
}

func TestWalReplayFromZeroAfterTruncatedWalWithFlushState(t *testing.T) {
	dir := t.TempDir()
	m, err := manifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.AppendNewFile(1); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	walPath := filepath.Join(dir, "wal.log")
	if err := os.WriteFile(walPath, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeWalFlushState(dir, walFlushState{FreezeOffset: 100, SSTID: 1}); err != nil {
		t.Fatal(err)
	}
	sstPath := filepath.Join(dir, "sst_00000001.sst")
	w, err := sstable.NewWriter(sstPath, 4096, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Add([]byte("flushed"), []byte("in-sst"), false); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Put([]byte("after"), []byte("open")); err != nil {
		t.Fatal(err)
	}
	val, err := db.Get([]byte("after"))
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "open" {
		t.Fatalf("got %q", val)
	}
}

func TestWalReplayStartOffsetUnknownSST(t *testing.T) {
	dir := t.TempDir()
	m, err := manifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	if err := writeWalFlushState(dir, walFlushState{FreezeOffset: 50, SSTID: 99}); err != nil {
		t.Fatal(err)
	}

	db := &DB{dir: dir, manifest: m}
	off, err := db.walReplayStartOffset()
	if err != nil {
		t.Fatal(err)
	}
	if off != 0 {
		t.Fatalf("offset = %d, want 0 when flushed SST not in manifest", off)
	}
}

func TestWalFlushStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st := walFlushState{FreezeOffset: 256, SSTID: 42}
	if err := writeWalFlushState(dir, st); err != nil {
		t.Fatal(err)
	}
	got, ok, err := readWalFlushState(dir)
	if err != nil || !ok {
		t.Fatalf("read: ok=%v err=%v", ok, err)
	}
	if got != st {
		t.Fatalf("got %+v, want %+v", got, st)
	}
	if err := removeWalFlushState(dir); err != nil {
		t.Fatal(err)
	}
	_, ok, err = readWalFlushState(dir)
	if err != nil || ok {
		t.Fatalf("after remove: ok=%v err=%v", ok, err)
	}
}

