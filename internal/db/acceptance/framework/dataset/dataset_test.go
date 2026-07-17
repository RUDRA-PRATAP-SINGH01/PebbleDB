package dataset

import (
	"bytes"
	"context"
	"fmt"
	"testing"
)

// MockLogicalWriter tracks operations to assert generator correctness.
type MockLogicalWriter struct {
	Puts    map[string][]byte
	Deletes map[string]bool
	Synced  bool
	Closed  bool
}

func NewMockLogicalWriter() *MockLogicalWriter {
	return &MockLogicalWriter{
		Puts:    make(map[string][]byte),
		Deletes: make(map[string]bool),
	}
}

func (m *MockLogicalWriter) Put(key, value []byte) error {
	m.Puts[string(key)] = value
	return nil
}

func (m *MockLogicalWriter) Delete(key []byte) error {
	m.Deletes[string(key)] = true
	delete(m.Puts, string(key))
	return nil
}

func (m *MockLogicalWriter) Flush() error {
	return nil
}

func (m *MockLogicalWriter) Sync() error {
	m.Synced = true
	return nil
}

func (m *MockLogicalWriter) Close() error {
	m.Closed = true
	return nil
}

func TestSequentialGeneratorDeterministicState(t *testing.T) {
	ctx := context.Background()
	writer := NewMockLogicalWriter()

	// 50 keys, 5 random overwrites, delete every 5th key
	gen := NewSequentialGenerator(42, 50, 5, 5)
	
	stateVal, err := gen.Generate(ctx, writer)
	if err != nil {
		t.Fatalf("dataset generation failed: %v", err)
	}

	expected, ok := stateVal.(*MapExpectedState)
	if !ok {
		t.Fatalf("expected *MapExpectedState, got %T", stateVal)
	}

	// 1. Verify key count
	if len(expected.State) != 50 {
		t.Fatalf("expected state size 50, got %d", len(expected.State))
	}

	// 2. Verify deleted keys
	for i := 0; i < 50; i += 5 {
		key := fmtKey(i)
		snap, exists := expected.Get(key)
		if !exists {
			t.Fatalf("key %s should exist in expected state tracking", string(key))
		}
		if !snap.Tombstone {
			t.Fatalf("key %s should be flagged as tombstone in expected state", string(key))
		}
		if writer.Deletes[string(key)] != true {
			t.Fatalf("key %s should have been written to Delete on logical writer", string(key))
		}
	}

	// 3. Verify sync occurred
	if !writer.Synced {
		t.Fatal("generator did not call Sync on logical writer")
	}

	// 4. Verify value retrieval matches
	for i := 0; i < 50; i++ {
		if i%5 == 0 {
			continue // skip deleted keys
		}
		key := fmtKey(i)
		snap, exists := expected.Get(key)
		if !exists {
			t.Fatalf("expected key %s not found in map", string(key))
		}
		if snap.Tombstone {
			t.Fatalf("key %s should not be flagged as tombstone", string(key))
		}
		if !bytes.Equal(writer.Puts[string(key)], snap.Value) {
			t.Fatalf("logical writer value does not match expected state for key %s", string(key))
		}
	}
}

func fmtKey(i int) []byte {
	return []byte(fmt.Sprintf("key_%08d", i))
}
