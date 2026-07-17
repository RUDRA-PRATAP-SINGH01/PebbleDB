// Package dataset implements the data generators and logical state expectations
// for PebbleDB acceptance test verification.
//
// Dependency Rules:
// - Imports: interfaces, types.
package dataset

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/interfaces"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// MapExpectedState implements a lookup map for validation of recovered logical state.
type MapExpectedState struct {
	State map[string]types.ValueSnapshot
}

// NewMapExpectedState allocates an empty MapExpectedState.
func NewMapExpectedState() *MapExpectedState {
	return &MapExpectedState{
		State: make(map[string]types.ValueSnapshot),
	}
}

// Get queries expected status of a key.
func (m *MapExpectedState) Get(key []byte) (types.ValueSnapshot, bool) {
	snap, exists := m.State[string(key)]
	return snap, exists
}

// Keys returns all keys tracked by the expected state.
func (m *MapExpectedState) Keys() [][]byte {
	keys := make([][]byte, 0, len(m.State))
	for k := range m.State {
		keys = append(keys, []byte(k))
	}
	return keys
}

// SequentialGenerator generates deterministic sequential key sequences with optional overwrites and deletes.
type SequentialGenerator struct {
	Seed           int64
	Count          int
	OverwriteCount int
	TombstoneEvery int
}

// NewSequentialGenerator allocates a new SequentialGenerator.
func NewSequentialGenerator(seed int64, count int, overwriteCount int, tombstoneEvery int) *SequentialGenerator {
	return &SequentialGenerator{
		Seed:           seed,
		Count:          count,
		OverwriteCount: overwriteCount,
		TombstoneEvery: tombstoneEvery,
	}
}

// Generate implements the interfaces.Dataset interface, writing to the database using LogicalWriter.
func (g *SequentialGenerator) Generate(ctx context.Context, wr interface{}) (interface{}, error) {
	writer, ok := wr.(interfaces.LogicalWriter)
	if !ok {
		return nil, fmt.Errorf("dataset generator requires interfaces.LogicalWriter, got %T", wr)
	}

	rng := rand.New(rand.NewSource(g.Seed))
	expected := NewMapExpectedState()

	// 1. Initial write run
	for i := 0; i < g.Count; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		key := []byte(fmt.Sprintf("key_%08d", i))
		val := []byte(fmt.Sprintf("value_%08d_%016x", i, rng.Uint64()))

		if err := writer.Put(key, val); err != nil {
			return nil, err
		}

		expected.State[string(key)] = types.ValueSnapshot{
			Value:     val,
			Tombstone: false,
			Version:   1,
		}
	}

	// 2. Overwrite runs
	for i := 0; i < g.OverwriteCount; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// select a random key index to overwrite
		idx := rng.Intn(g.Count)
		key := []byte(fmt.Sprintf("key_%08d", idx))
		val := []byte(fmt.Sprintf("overwrite_%08d_%016x", idx, rng.Uint64()))

		if err := writer.Put(key, val); err != nil {
			return nil, err
		}

		expected.State[string(key)] = types.ValueSnapshot{
			Value:     val,
			Tombstone: false,
			Version:   2,
		}
	}

	// 3. Tombstone write run (Delete)
	if g.TombstoneEvery > 0 {
		for i := 0; i < g.Count; i += g.TombstoneEvery {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			key := []byte(fmt.Sprintf("key_%08d", i))
			if err := writer.Delete(key); err != nil {
				return nil, err
			}

			expected.State[string(key)] = types.ValueSnapshot{
				Value:     nil,
				Tombstone: true,
				Version:   3,
			}
		}
	}

	// Force WAL and Memtable Sync
	if err := writer.Sync(); err != nil {
		return nil, err
	}

	return expected, nil
}

// KeyCount returns total unique keys written.
func (g *SequentialGenerator) KeyCount() int {
	return g.Count
}
