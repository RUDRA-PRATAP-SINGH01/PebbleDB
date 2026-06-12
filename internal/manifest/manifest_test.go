package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifestNewFileAndReplay(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.AppendNewFile(1); err != nil {
		t.Fatal(err)
	}
	if err := m.AppendNewFile(3); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	m2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()

	ids := m2.LiveIDs()
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 3 {
		t.Fatalf("live ids = %v, want [1 3]", ids)
	}
}

func TestManifestSetFileSetReplaces(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := m.AppendNewFile(1); err != nil {
		t.Fatal(err)
	}
	if err := m.AppendNewFile(2); err != nil {
		t.Fatal(err)
	}
	if err := m.AppendSetFileSet([]uint64{5}); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	ids, err := ReplayFile(filepath.Join(dir, initialManifest))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != 5 {
		t.Fatalf("replay ids = %v, want [5]", ids)
	}
}

func TestCurrentFileWritten(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m.Close()

	data, err := os.ReadFile(filepath.Join(dir, currentFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != initialManifest+"\n" {
		t.Fatalf("CURRENT = %q, want %q\n", data, initialManifest+"\n")
	}
}

func TestManifestSalvagesTrailingPartialRecord(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.AppendNewFile(1); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, initialManifest)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0, 0, 0, 2, 0xAA, 0xBB}); err != nil {
		t.Fatal(err)
	}
	f.Close()

	m2, err := Open(dir)
	if err != nil {
		t.Fatalf("open after partial tail: %v", err)
	}
	defer m2.Close()

	ids := m2.LiveIDs()
	if len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("live ids = %v, want [1]", ids)
	}
}

func TestManifestMaybeCompactRotates(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	for i := uint64(1); i <= 5; i++ {
		if err := m.AppendNewFile(i); err != nil {
			t.Fatal(err)
		}
	}
	// Force compaction regardless of threshold.
	m.mu.Lock()
	m.recordCount = compactRecordThreshold
	m.mu.Unlock()

	if err := m.MaybeCompact(); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	current, err := readCurrentManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if current != "MANIFEST-000002" {
		t.Fatalf("CURRENT = %q, want MANIFEST-000002", current)
	}
	if _, err := os.Stat(filepath.Join(dir, initialManifest)); err == nil {
		t.Fatal("old manifest should be removed after rotation")
	}

	m2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	ids := m2.LiveIDs()
	if len(ids) != 5 {
		t.Fatalf("live ids after rotation = %v, want 5 entries", ids)
	}
}
