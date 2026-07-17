// Package resource manages CPU/memory/FD budgets and isolated temp directories for ATF runs.
package resource

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/errors"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// Reservation is an opaque, pointer-stable grant from Reserve.
type Reservation struct {
	ID        uint64
	Request   types.ResourceRequest
	GrantedAt time.Time
	released  atomic.Bool
}

// ResourceManager tracks resource budgets and sandbox directories.
type ResourceManager struct {
	mu                sync.Mutex
	cond              *sync.Cond
	logger            *logging.Logger
	baseDir           string
	maxCPUs           int
	maxMemoryMB       int64
	maxFDs            int
	allocatedCPUs     int
	allocatedMemoryMB int64
	allocatedFDs      int
	nextID            uint64
	retainArtifacts   bool
}

// NewResourceManager initializes a ResourceManager.
func NewResourceManager(
	logger *logging.Logger,
	baseDir string,
	maxCPUs int,
	maxMemoryMB int64,
	maxFDs int,
) *ResourceManager {
	rm := &ResourceManager{
		logger:          logger,
		baseDir:         baseDir,
		maxCPUs:         maxCPUs,
		maxMemoryMB:     maxMemoryMB,
		maxFDs:          maxFDs,
		retainArtifacts: true,
	}
	rm.cond = sync.NewCond(&rm.mu)
	return rm
}

// SetRetainArtifacts controls whether failed-run directories are kept.
func (rm *ResourceManager) SetRetainArtifacts(retain bool) {
	rm.mu.Lock()
	rm.retainArtifacts = retain
	rm.mu.Unlock()
}

// Reserve blocks until the request fits the budget or ctx is canceled.
func (rm *ResourceManager) Reserve(ctx context.Context, req types.ResourceRequest) (*Reservation, error) {
	if req.CPUs < 0 || req.MemoryMB < 0 || req.FileDescriptor < 0 {
		return nil, errors.NewResourceError("resource request fields must be non-negative", nil)
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	for !rm.canAllocateLocked(req) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Wake waiters when ctx cancels so we do not deadlock.
		stop := context.AfterFunc(ctx, func() {
			rm.mu.Lock()
			rm.cond.Broadcast()
			rm.mu.Unlock()
		})
		rm.cond.Wait()
		stop()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}

	rm.allocatedCPUs += req.CPUs
	rm.allocatedMemoryMB += req.MemoryMB
	rm.allocatedFDs += req.FileDescriptor
	rm.nextID++
	id := rm.nextID

	rm.logger.Debug("Allocated resources id=%d CPUs=%d/%d Memory=%d/%dMB FDs=%d/%d",
		id, rm.allocatedCPUs, rm.maxCPUs, rm.allocatedMemoryMB, rm.maxMemoryMB, rm.allocatedFDs, rm.maxFDs)

	return &Reservation{
		ID:        id,
		Request:   req,
		GrantedAt: time.Now(),
	}, nil
}

func (rm *ResourceManager) canAllocateLocked(req types.ResourceRequest) bool {
	return rm.allocatedCPUs+req.CPUs <= rm.maxCPUs &&
		rm.allocatedMemoryMB+req.MemoryMB <= rm.maxMemoryMB &&
		rm.allocatedFDs+req.FileDescriptor <= rm.maxFDs
}

// Release returns a reservation to the pool. Idempotent.
func (rm *ResourceManager) Release(res *Reservation) error {
	if res == nil {
		return errors.NewResourceError("nil reservation", nil)
	}
	if !res.released.CompareAndSwap(false, true) {
		return nil
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.allocatedCPUs -= res.Request.CPUs
	rm.allocatedMemoryMB -= res.Request.MemoryMB
	rm.allocatedFDs -= res.Request.FileDescriptor
	if rm.allocatedCPUs < 0 || rm.allocatedMemoryMB < 0 || rm.allocatedFDs < 0 {
		return errors.NewResourceError("resource accounting underflow", nil)
	}

	rm.logger.Debug("Released resources id=%d remaining CPUs=%d/%d Memory=%d/%dMB",
		res.ID, rm.allocatedCPUs, rm.maxCPUs, rm.allocatedMemoryMB, rm.maxMemoryMB)
	rm.cond.Broadcast()
	return nil
}

// AllocateTempDir creates an isolated subdirectory under baseDir.
func (rm *ResourceManager) AllocateTempDir(prefix string) (string, error) {
	prefix = sanitizePrefix(prefix)
	if err := os.MkdirAll(rm.baseDir, 0755); err != nil {
		return "", errors.NewLockError("failed to create base directory", err)
	}

	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		return "", errors.NewResourceError("failed to generate random suffix for temp dir", err)
	}

	dirName := fmt.Sprintf("%s_%s_%s", prefix, time.Now().Format("20060102_150405"), hex.EncodeToString(randBytes))
	dirPath := filepath.Join(rm.baseDir, dirName)
	clean := filepath.Clean(dirPath)
	if !strings.HasPrefix(clean, filepath.Clean(rm.baseDir)+string(os.PathSeparator)) && clean != filepath.Clean(rm.baseDir) {
		return "", errors.NewResourceError("refusing path outside base dir", nil)
	}

	if err := os.MkdirAll(clean, 0755); err != nil {
		return "", errors.NewResourceError("failed to create target temp directory", err)
	}
	rm.logger.Debug("Allocated isolated directory: %s", clean)
	return clean, nil
}

func sanitizePrefix(prefix string) string {
	prefix = filepath.Base(prefix)
	prefix = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, prefix)
	if prefix == "" || prefix == "." || prefix == ".." {
		return "run"
	}
	return prefix
}

// CleanTempDir removes path. For retainArtifacts, use RetainOrClean instead.
func (rm *ResourceManager) CleanTempDir(path string) error {
	rm.logger.Debug("Sweeping directory: %s", path)
	return os.RemoveAll(path)
}

// RetainOrClean keeps path on failure when retainArtifacts is set; otherwise deletes it.
func (rm *ResourceManager) RetainOrClean(path string, passed bool) error {
	rm.mu.Lock()
	retain := rm.retainArtifacts
	rm.mu.Unlock()
	if !passed && retain {
		rm.logger.Warn("Retaining artifact directory after failure: %s", path)
		return nil
	}
	return rm.CleanTempDir(path)
}
