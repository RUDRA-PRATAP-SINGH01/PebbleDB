package resource

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

func TestReserveReleaseIdempotent(t *testing.T) {
	rm := NewResourceManager(logging.NewLogger(nil, logging.LevelError), t.TempDir(), 2, 128, 20)
	ctx := context.Background()
	r, err := rm.Reserve(ctx, types.ResourceRequest{CPUs: 1, MemoryMB: 10, FileDescriptor: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := rm.Release(r); err != nil {
		t.Fatal(err)
	}
	if err := rm.Release(r); err != nil {
		t.Fatal(err)
	}
}

func TestReserveFairWakeup(t *testing.T) {
	rm := NewResourceManager(logging.NewLogger(nil, logging.LevelError), t.TempDir(), 1, 64, 10)
	ctx := context.Background()
	r1, err := rm.Reserve(ctx, types.ResourceRequest{CPUs: 1, MemoryMB: 1, FileDescriptor: 1})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var secondErr error
	go func() {
		defer wg.Done()
		_, secondErr = rm.Reserve(context.Background(), types.ResourceRequest{CPUs: 1, MemoryMB: 1, FileDescriptor: 1})
	}()

	time.Sleep(30 * time.Millisecond)
	if err := rm.Release(r1); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if secondErr != nil {
		t.Fatalf("second reserve: %v", secondErr)
	}
}

func TestAllocateTempDirRejectsTraversal(t *testing.T) {
	base := t.TempDir()
	rm := NewResourceManager(logging.NewLogger(nil, logging.LevelError), base, 1, 64, 10)
	dir, err := rm.AllocateTempDir("../../../evil")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(dir) != base && filepath.Clean(filepath.Dir(dir)) != filepath.Clean(base) {
		// must remain under base
		rel, relErr := filepath.Rel(base, dir)
		if relErr != nil || len(rel) >= 2 && rel[:2] == ".." {
			t.Fatalf("path escaped base: %s", dir)
		}
	}
}
