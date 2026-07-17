package crash

import (
	"fmt"
	"sort"
	"sync"
)

// entry binds a crash point definition to its hook.
type entry struct {
	point CrashPoint
	hook  CrashHook
}

// Registry is a thread-safe catalog of crash points and hooks.
type Registry struct {
	mu     sync.RWMutex
	byID   map[string]entry
	byHook map[string]string // EngineHook → ID
	order  []string
}

// NewRegistry allocates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		byID:   make(map[string]entry),
		byHook: make(map[string]string),
	}
}

// Register adds a crash point and its hook. Duplicate IDs or EngineHook values are rejected.
func (r *Registry) Register(point CrashPoint, hook CrashHook) error {
	if err := point.Validate(); err != nil {
		return err
	}
	if hook == nil {
		return newError(ErrInvalidConfig, "hook must not be nil", nil)
	}
	if hook.ID() != point.ID {
		return newError(ErrInvalidConfig, fmt.Sprintf("hook ID %q must match point ID %q", hook.ID(), point.ID), nil)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byID[point.ID]; exists {
		return newError(ErrDuplicatePoint, fmt.Sprintf("crash point %q already registered", point.ID), nil)
	}
	if other, exists := r.byHook[point.EngineHook]; exists {
		return newError(ErrDuplicatePoint, fmt.Sprintf("engine hook %q already registered by %q", point.EngineHook, other), nil)
	}

	cloned := point.Clone()
	r.byID[point.ID] = entry{point: cloned, hook: hook}
	r.byHook[point.EngineHook] = point.ID
	r.order = append(r.order, point.ID)
	return nil
}

// Lookup returns a crash point by ID.
func (r *Registry) Lookup(id string) (CrashPoint, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.byID[id]
	if !ok {
		return CrashPoint{}, false
	}
	return e.point.Clone(), true
}

// LookupByEngineHook resolves a point by PEBBLEDB_CRASH_AT value.
func (r *Registry) LookupByEngineHook(hook string) (CrashPoint, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byHook[hook]
	if !ok {
		return CrashPoint{}, false
	}
	return r.byID[id].point.Clone(), true
}

// Resolve accepts either a registry ID or an EngineHook alias.
func (r *Registry) Resolve(idOrHook string) (CrashPoint, CrashHook, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if e, ok := r.byID[idOrHook]; ok {
		return e.point.Clone(), e.hook, nil
	}
	if id, ok := r.byHook[idOrHook]; ok {
		e := r.byID[id]
		return e.point.Clone(), e.hook, nil
	}
	return CrashPoint{}, nil, newError(ErrUnknownPoint, fmt.Sprintf("unknown crash point %q", idOrHook), nil)
}

// LookupByPhase returns all points in the given phase (sorted by ID).
func (r *Registry) LookupByPhase(phase Phase) []CrashPoint {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []CrashPoint
	for _, id := range r.order {
		e := r.byID[id]
		if e.point.Phase == phase {
			out = append(out, e.point.Clone())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// LookupByCategory returns all points in the given category (sorted by ID).
func (r *Registry) LookupByCategory(cat Category) []CrashPoint {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []CrashPoint
	for _, id := range r.order {
		e := r.byID[id]
		if e.point.Category == cat {
			out = append(out, e.point.Clone())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ValidateDependencies ensures every point's Dependencies refer to registered IDs.
func (r *Registry) ValidateDependencies() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, id := range r.order {
		e := r.byID[id]
		for _, dep := range e.point.Dependencies {
			if _, ok := r.byID[dep]; !ok {
				return newError(ErrDependency, fmt.Sprintf("point %q depends on missing %q", id, dep), nil)
			}
		}
	}
	return nil
}

// List returns all registered crash points in registration order.
func (r *Registry) List() []CrashPoint {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]CrashPoint, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.byID[id].point.Clone())
	}
	return out
}

// Capabilities returns capability projections for all registered points.
func (r *Registry) Capabilities() []Capability {
	points := r.List()
	out := make([]Capability, len(points))
	for i, p := range points {
		out[i] = CapabilityOf(p)
	}
	return out
}

// Len returns the number of registered crash points.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.order)
}

// Hook returns the hook for a crash point ID.
func (r *Registry) Hook(id string) (CrashHook, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.byID[id]
	return e.hook, ok
}
