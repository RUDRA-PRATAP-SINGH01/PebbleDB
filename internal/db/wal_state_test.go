package db

import (
	"os"
	"testing"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/manifest"
)

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

	db := &DB{dir: dir, manifest: m}
	defer m.Close()

	off, err := db.walReplayStartOffset()
	if err != nil {
		t.Fatal(err)
	}
	if off != 128 {
		t.Fatalf("replay offset = %d, want 128", off)
	}
}

func TestGetReturnsBackgroundError(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.setBackgroundErr("compaction", os.ErrPermission)
	_, err = db.Get([]byte("k"))
	if err == nil {
		t.Fatal("expected background error on Get")
	}
}

func TestScanReturnsBackgroundError(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.setBackgroundErr("flush", os.ErrPermission)
	_, err = db.Scan(nil, nil)
	if err == nil {
		t.Fatal("expected background error on Scan")
	}
}
