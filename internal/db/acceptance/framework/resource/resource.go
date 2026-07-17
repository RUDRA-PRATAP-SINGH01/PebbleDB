// Package resource implements the system resource manager, controlling concurrent allocations
// of CPU cores, memory limits, file descriptor budgets, and isolated database directories.
//
// Dependency Rules:
// - Imports: interfaces, types, errors, logging.
package resource

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/errors"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// ResourceManager manages resource allocations and isolated path allocations.
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
}

// NewResourceManager initializes a new ResourceManager with resource limit boundaries.
func NewResourceManager(
	logger *logging.Logger,
	baseDir string,
	maxCPUs int,
	maxMemoryMB int64,
	maxFDs int,
) *ResourceManager {
	rm := &ResourceManager{
		logger:      logger,
		baseDir:     baseDir,
		maxCPUs:     maxCPUs,
		maxMemoryMB: maxMemoryMB,
		maxFDs:      maxFDs,
	}
	rm.cond = sync.NewCond(&rm.mu)
	return rm
}

// Reserve allocates system resources matching the request. Blocks until slots are available.
func (rm *ResourceManager) Reserve(ctx context.Context, req interface{}) (interface{}, error) {
	request, ok := req.(types.ResourceRequest)
	if !ok {
		return nil, errors.NewResourceError("invalid resource request type", nil)
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Wait loop with context checking
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if rm.canAllocateLocked(request) {
			rm.allocatedCPUs += request.CPUs
			rm.allocatedMemoryMB += request.MemoryMB
			rm.allocatedFDs += request.FileDescriptor

			rm.logger.Debug("Allocated resources: CPUs=%d (total %d/%d), Memory=%dMB (total %d/%dMB)",
				request.CPUs, rm.allocatedCPUs, rm.maxCPUs,
				request.MemoryMB, rm.allocatedMemoryMB, rm.maxMemoryMB)

			return types.ResourceAllocation{
				Request:   request,
				GrantedAt: time.Now(),
				Released:  false,
			}, nil
		}

		// Wait for resource release signal
		releasedSignal := make(chan struct{})
		go func() {
			rm.mu.Lock()
			defer rm.mu.Unlock()
			rm.cond.Wait()
			close(releasedSignal)
		}()

		rm.mu.Unlock()
		select {
		case <-releasedSignal:
			rm.mu.Lock() // Re-acquire lock for next check loop
		case <-ctx.Done():
			rm.mu.Lock() // Re-acquire lock to satisfy defer
			return nil, ctx.Err()
		}
	}
}

func (rm *ResourceManager) canAllocateLocked(req types.ResourceRequest) bool {
	if rm.allocatedCPUs+req.CPUs > rm.maxCPUs {
		return false
	}
	if rm.allocatedMemoryMB+req.MemoryMB > rm.maxMemoryMB {
		return false
	}
	if rm.allocatedFDs+req.FileDescriptor > rm.maxFDs {
		return false
	}
	return true
}

// Release returns resource slots back to the system pool.
func (rm *ResourceManager) Release(alloc interface{}) error {
	allocation, ok := alloc.(types.ResourceAllocation)
	if !ok {
		return errors.NewResourceError("invalid resource allocation type", nil)
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	if allocation.Released {
		return nil
	}

	rm.allocatedCPUs -= allocation.Request.CPUs
	rm.allocatedMemoryMB -= allocation.Request.MemoryMB
	rm.allocatedFDs -= allocation.Request.FileDescriptor

	rm.logger.Debug("Released resources: CPUs=%d (remaining %d/%d), Memory=%dMB (remaining %d/%dMB)",
		allocation.Request.CPUs, rm.allocatedCPUs, rm.maxCPUs,
		allocation.Request.MemoryMB, rm.allocatedMemoryMB, rm.maxMemoryMB)

	rm.cond.Broadcast() // Signal waiting scenarios
	return nil
}

// AllocateTempDir creates a uniquely hashed subdirectory namespace for isolated database runs.
func (rm *ResourceManager) AllocateTempDir(prefix string) (string, error) {
	if err := os.MkdirAll(rm.baseDir, 0755); err != nil {
		return "", errors.NewLockError("failed to create base directory", err)
	}

	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		return "", errors.NewResourceError("failed to generate random suffix for temp dir", err)
	}

	dirName := fmt.Sprintf("%s_%s_%s", prefix, time.Now().Format("20060102_150405"), hex.EncodeToString(randBytes))
	dirPath := filepath.Join(rm.baseDir, dirName)

	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", errors.NewResourceError("failed to create target temp directory", err)
	}

	rm.logger.Debug("Allocated isolated directory: %s", dirPath)
	return dirPath, nil
}

// CleanTempDir wipes the target database directory.
func (rm *ResourceManager) CleanTempDir(path string) error {
	rm.logger.Debug("Sweeping directory: %s", path)
	return os.RemoveAll(path)
}
