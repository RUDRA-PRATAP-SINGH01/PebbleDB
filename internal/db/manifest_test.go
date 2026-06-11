package db

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/manifest"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/sstable"
)

func TestManifestIgnoresOrphanSSTAfterCompactionCrash(t *testing.T) {
	dir := t.TempDir()

	db1, err := Open(Options{
		Dir:                   dir,
		MemtableSize: 8,
		CompactionThreshold:   2,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := db1.Put([]byte("A"), []byte("1")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := db1.Put([]byte(fmt.Sprintf("k%02d", i)), []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	waitForFlush(t, db1)

	if err := db1.Delete([]byte("A")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := db1.Put([]byte(fmt.Sprintf("z%02d", i)), []byte("y")); err != nil {
			t.Fatal(err)
		}
	}
	waitForFlush(t, db1)
	waitForCompaction(t, db1, 1)

	// Simulate post-compaction crash: obsolete SST left on disk, manifest committed merged only.
	orphanPath := filepath.Join(dir, "sst_00000001.sst")
	w, err := sstable.NewWriter(orphanPath, 4096, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Add([]byte("A"), []byte("stale"), false); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	if err := db1.Close(); err != nil {
		t.Fatal(err)
	}

	db2, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	_, err = db2.Get([]byte("A"))
	if err != ErrNotFound {
		t.Fatalf("Get(A) after orphan SST = %v, want ErrNotFound", err)
	}

	m, err := manifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if m.Contains(1) {
		t.Error("manifest should not list orphan sst id 1")
	}
}

func TestFlushWritesManifestRecord(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir, MemtableSize: flushTestThreshold})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		if err := db.Put([]byte(fmt.Sprintf("k%d", i)), []byte("v")); err != nil {
			t.Fatal(err)
		}
	}
	waitForFlush(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	ids, err := manifest.ReplayFile(filepath.Join(dir, "MANIFEST-000001"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("expected manifest NewFile records after flush")
	}
}
