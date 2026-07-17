// Package dataset generates deterministic workloads and persists expected state for ATF.
package dataset

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/interfaces"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

const ExpectedStateFileName = "expected_state.json"

// MapExpectedState is the ground-truth logical state after Generate.
type MapExpectedState struct {
	Seed  int64                          `json:"seed"`
	Count int                            `json:"count"`
	State map[string]types.ValueSnapshot `json:"state"`
}

// NewMapExpectedState allocates an empty map.
func NewMapExpectedState(seed int64, count int) *MapExpectedState {
	return &MapExpectedState{
		Seed:  seed,
		Count: count,
		State: make(map[string]types.ValueSnapshot),
	}
}

// Get returns the snapshot for key.
func (m *MapExpectedState) Get(key []byte) (types.ValueSnapshot, bool) {
	snap, ok := m.State[string(key)]
	return snap, ok
}

// Keys returns sorted keys for deterministic verification.
func (m *MapExpectedState) Keys() [][]byte {
	keys := make([]string, 0, len(m.State))
	for k := range m.State {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([][]byte, len(keys))
	for i, k := range keys {
		out[i] = []byte(k)
	}
	return out
}

// Persist writes expected state atomically into dir/expected_state.json.
func (m *MapExpectedState) Persist(dir string) error {
	path := filepath.Join(dir, ExpectedStateFileName)
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// LoadExpectedState reads expected_state.json from dir.
func LoadExpectedState(dir string) (*MapExpectedState, error) {
	path := filepath.Join(dir, ExpectedStateFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m MapExpectedState
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.State == nil {
		m.State = make(map[string]types.ValueSnapshot)
	}
	return &m, nil
}

// SequentialGenerator writes a deterministic keyspace with overwrites and tombstones.
type SequentialGenerator struct {
	Seed           int64
	Count          int
	OverwriteCount int
	TombstoneEvery int
}

// NewSequentialGenerator constructs a generator.
func NewSequentialGenerator(seed int64, count, overwriteCount, tombstoneEvery int) *SequentialGenerator {
	return &SequentialGenerator{
		Seed:           seed,
		Count:          count,
		OverwriteCount: overwriteCount,
		TombstoneEvery: tombstoneEvery,
	}
}

// KeyCount returns unique key cardinality.
func (g *SequentialGenerator) KeyCount() int { return g.Count }

// Generate writes through LogicalWriter and returns expected state (not yet persisted).
func (g *SequentialGenerator) Generate(ctx context.Context, writer interfaces.LogicalWriter) (*MapExpectedState, error) {
	if g.Count <= 0 {
		return nil, fmt.Errorf("dataset: count must be positive")
	}
	rng := rand.New(rand.NewSource(g.Seed))
	expected := NewMapExpectedState(g.Seed, g.Count)

	put := func(key, val []byte, version uint64) error {
		if err := writer.Put(key, val); err != nil {
			return err
		}
		expected.State[string(key)] = types.ValueSnapshot{
			Value:     append([]byte(nil), val...),
			Tombstone: false,
			Version:   version,
		}
		return nil
	}

	for i := 0; i < g.Count; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key := fmtKey(i)
		val := []byte(fmt.Sprintf("value_%08d_%016x", i, rng.Uint64()))
		if err := put(key, val, 1); err != nil {
			return nil, err
		}
	}

	for i := 0; i < g.OverwriteCount; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		idx := rng.Intn(g.Count)
		key := fmtKey(idx)
		prev := expected.State[string(key)]
		nextVer := prev.Version + 1
		if nextVer < 2 {
			nextVer = 2
		}
		val := []byte(fmt.Sprintf("overwrite_%08d_%016x", idx, rng.Uint64()))
		if err := put(key, val, nextVer); err != nil {
			return nil, err
		}
	}

	if g.TombstoneEvery > 0 {
		for i := 0; i < g.Count; i += g.TombstoneEvery {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			key := fmtKey(i)
			if err := writer.Delete(key); err != nil {
				return nil, err
			}
			prev := expected.State[string(key)]
			expected.State[string(key)] = types.ValueSnapshot{
				Value:     nil,
				Tombstone: true,
				Version:   prev.Version + 1,
			}
		}
	}

	if err := writer.Sync(); err != nil {
		return nil, err
	}
	return expected, nil
}

func fmtKey(i int) []byte {
	return []byte(fmt.Sprintf("key_%08d", i))
}
