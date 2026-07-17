package dataset

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
)

type MockLogicalWriter struct {
	Puts    map[string][]byte
	Deletes map[string]bool
	Synced  bool
	Flushed bool
	Closed  bool
}

func NewMockLogicalWriter() *MockLogicalWriter {
	return &MockLogicalWriter{
		Puts:    make(map[string][]byte),
		Deletes: make(map[string]bool),
	}
}

func (m *MockLogicalWriter) Put(key, value []byte) error {
	m.Puts[string(key)] = append([]byte(nil), value...)
	delete(m.Deletes, string(key))
	return nil
}

func (m *MockLogicalWriter) Delete(key []byte) error {
	m.Deletes[string(key)] = true
	delete(m.Puts, string(key))
	return nil
}

func (m *MockLogicalWriter) Flush() error { m.Flushed = true; return nil }
func (m *MockLogicalWriter) Sync() error  { m.Synced = true; return nil }
func (m *MockLogicalWriter) Close() error { m.Closed = true; return nil }

func TestSequentialGeneratorDeterministicState(t *testing.T) {
	ctx := context.Background()
	writer := NewMockLogicalWriter()
	gen := NewSequentialGenerator(42, 50, 5, 5)

	expected, err := gen.Generate(ctx, writer)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(expected.State) != 50 {
		t.Fatalf("state size=%d want 50", len(expected.State))
	}
	for i := 0; i < 50; i += 5 {
		key := fmtKey(i)
		snap, ok := expected.Get(key)
		if !ok || !snap.Tombstone {
			t.Fatalf("key %s should be tombstone", key)
		}
	}
	if !writer.Synced {
		t.Fatal("expected Sync")
	}

	// Determinism: same seed → same values
	writer2 := NewMockLogicalWriter()
	expected2, err := NewSequentialGenerator(42, 50, 5, 5).Generate(ctx, writer2)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range expected.Keys() {
		a, _ := expected.Get(key)
		b, _ := expected2.Get(key)
		if a.Tombstone != b.Tombstone || a.Version != b.Version || !bytes.Equal(a.Value, b.Value) {
			t.Fatalf("non-deterministic state for %s", key)
		}
	}
}

func TestExpectedStatePersistRoundTrip(t *testing.T) {
	ctx := context.Background()
	expected, err := NewSequentialGenerator(7, 20, 3, 4).Generate(ctx, NewMockLogicalWriter())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := expected.Persist(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadExpectedState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Seed != 7 || loaded.Count != 20 {
		t.Fatalf("meta mismatch: %+v", loaded)
	}
	if len(loaded.State) != len(expected.State) {
		t.Fatalf("len %d vs %d", len(loaded.State), len(expected.State))
	}
	for _, key := range expected.Keys() {
		a, _ := expected.Get(key)
		b, _ := loaded.Get(key)
		if a.Tombstone != b.Tombstone || !bytes.Equal(a.Value, b.Value) || a.Version != b.Version {
			t.Fatalf("mismatch %s: %+v vs %+v", key, a, b)
		}
	}
	if _, err := LoadExpectedState(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected error for missing dir")
	}
}
