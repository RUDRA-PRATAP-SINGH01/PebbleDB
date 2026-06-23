package manifest

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestManifestConcurrentCompactAndAppend stresses rotation while other goroutines
// append NewFile records. Before the C1 fix, appends during the unlocked
// rotateSnapshot window could land on a manifest file that was then deleted.
func TestManifestConcurrentCompactAndAppend(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	const (
		writers   = 8
		perWriter = 32
	)
	expected := make(map[uint64]struct{}, writers*perWriter)
	var expectedMu sync.Mutex

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(base uint64) {
			defer wg.Done()
			for i := uint64(0); i < perWriter; i++ {
				id := base*1000 + i + 1
				if err := m.AppendNewFile(id); err != nil {
					t.Errorf("AppendNewFile(%d): %v", id, err)
					return
				}
				expectedMu.Lock()
				expected[id] = struct{}{}
				expectedMu.Unlock()
			}
		}(uint64(w))
	}

	for c := 0; c < 6; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				if err := m.MaybeCompact(); err != nil {
					t.Errorf("MaybeCompact: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	inMemory := m.LiveIDs()
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	current, err := readCurrentManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	onDisk, err := ReplayFile(filepath.Join(dir, current))
	if err != nil {
		t.Fatal(err)
	}

	if len(inMemory) != len(expected) {
		t.Fatalf("in-memory live count = %d, want %d", len(inMemory), len(expected))
	}
	if len(onDisk) != len(expected) {
		t.Fatalf("on-disk live count = %d, want %d", len(onDisk), len(expected))
	}

	toSet := func(ids []uint64) map[uint64]struct{} {
		s := make(map[uint64]struct{}, len(ids))
		for _, id := range ids {
			s[id] = struct{}{}
		}
		return s
	}
	memSet := toSet(inMemory)
	diskSet := toSet(onDisk)

	for id := range expected {
		if _, ok := memSet[id]; !ok {
			t.Fatalf("id %d missing from in-memory live set after concurrent rotation", id)
		}
		if _, ok := diskSet[id]; !ok {
			t.Fatalf("id %d missing from on-disk manifest after concurrent rotation", id)
		}
	}
}

// TestManifestDoubleMaybeCompact verifies two concurrent compactions cannot
// interleave rotations (C3).
func TestManifestDoubleMaybeCompact(t *testing.T) {
	dir := t.TempDir()
	m, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	for i := uint64(1); i <= compactRecordThreshold+4; i++ {
		if err := m.AppendNewFile(i); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- m.MaybeCompact()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	ids := m.LiveIDs()
	if len(ids) != int(compactRecordThreshold+4) {
		t.Fatalf("live count = %d, want %d", len(ids), compactRecordThreshold+4)
	}

	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	current, err := readCurrentManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if current == "" {
		t.Fatal("CURRENT missing after double compaction")
	}
	if _, err := os.Stat(filepath.Join(dir, current)); err != nil {
		t.Fatalf("CURRENT manifest missing: %v", err)
	}
}
