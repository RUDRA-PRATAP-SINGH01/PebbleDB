package verifier

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds named verifier modules in deterministic registration order.
type Registry struct {
	mu        sync.RWMutex
	order     []string
	verifiers map[string]Verifier
}

// NewRegistry allocates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		verifiers: make(map[string]Verifier),
	}
}

// Register adds a verifier. Duplicate names are rejected.
func (r *Registry) Register(v Verifier) error {
	if v == nil {
		return fmt.Errorf("verifier: nil verifier")
	}
	name := v.Name()
	if name == "" {
		return fmt.Errorf("verifier: empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.verifiers[name]; exists {
		return fmt.Errorf("verifier: duplicate registration %q", name)
	}
	r.verifiers[name] = v
	r.order = append(r.order, name)
	return nil
}

// Get returns a verifier by name.
func (r *Registry) Get(name string) (Verifier, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.verifiers[name]
	return v, ok
}

// All returns verifiers in registration order.
func (r *Registry) All() []Verifier {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Verifier, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.verifiers[name])
	}
	return out
}

// Names returns registered verifier names in registration order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := append([]string(nil), r.order...)
	return out
}

// DefaultRegistry returns the production set of verifier modules in fixed order.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	for _, v := range []Verifier{
		MetadataVerifier{},
		GetVerifier{},
		IteratorVerifier{},
		RangeScanVerifier{},
		SnapshotVerifier{},
		DirectoryAudit{},
		ManifestAudit{},
		CheckpointAudit{},
	} {
		if err := r.Register(v); err != nil {
			panic(err)
		}
	}
	return r
}

// SortedNames returns names alphabetically (test helper for determinism checks).
func (r *Registry) SortedNames() []string {
	names := r.Names()
	sort.Strings(names)
	return names
}
