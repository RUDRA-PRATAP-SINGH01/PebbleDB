// Package registry implements a thread-safe Scenario registry, managing registrations,
// lookups, duplicate detection, and filters for campaign runs.
//
// Dependency Rules:
// - Imports: interfaces, types, errors.
package registry

import (
	"fmt"
	"regexp"
	"sort"
	"sync"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/errors"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/interfaces"
)

var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// Registry Filter configuration options.
type FilterOptions struct {
	Priority     int      `json:"priority,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// MapRegistry implements interfaces.ScenarioRegistry.
type MapRegistry struct {
	mu        sync.RWMutex
	scenarios map[string]interfaces.Scenario
}

// NewMapRegistry allocates an empty MapRegistry.
func NewMapRegistry() *MapRegistry {
	return &MapRegistry{
		scenarios: make(map[string]interfaces.Scenario),
	}
}

// Register adds a new Scenario to the registry. Validates formatting and checks duplicates.
func (r *MapRegistry) Register(scenario interfaces.Scenario) error {
	if scenario == nil {
		return errors.NewRegistrationError("scenario must not be nil", nil)
	}

	id := scenario.ID()
	if id == "" {
		return errors.NewRegistrationError("scenario ID must not be empty", nil)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 1. Duplicate detection
	if _, exists := r.scenarios[id]; exists {
		return errors.NewRegistrationError(fmt.Sprintf("scenario ID %s already registered", id), nil)
	}

	// 2. Version validation
	version := scenario.Version()
	if !semverPattern.MatchString(version) {
		return errors.NewRegistrationError(fmt.Sprintf("scenario %s has invalid semver: %q", id, version), nil)
	}

	// 3. Priority boundary checks
	priority := scenario.Priority()
	if priority < 1 || priority > 3 {
		return errors.NewRegistrationError(fmt.Sprintf("scenario %s has invalid priority: %d (want 1, 2, or 3)", id, priority), nil)
	}

	// 4. DAG dependency validation
	dag := scenario.VerificationDAG()
	for node, deps := range dag {
		if node == "" {
			return errors.NewRegistrationError(fmt.Sprintf("scenario %s DAG contains empty node key", id), nil)
		}
		for _, dep := range deps {
			if dep == "" {
				return errors.NewRegistrationError(fmt.Sprintf("scenario %s node %q dependency cannot be empty", id, node), nil)
			}
			if dep == node {
				return errors.NewRegistrationError(fmt.Sprintf("scenario %s node %q cannot depend on itself (circular reference)", id, node), nil)
			}
		}
	}

	r.scenarios[id] = scenario
	return nil
}

// Lookup retrieves a scenario by its unique ID string.
func (r *MapRegistry) Lookup(id string) (interfaces.Scenario, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sc, exists := r.scenarios[id]
	if !exists {
		return nil, errors.NewRegistrationError(fmt.Sprintf("scenario %s not found", id), nil)
	}
	return sc, nil
}

// List returns a sorted slice of all registered scenarios.
func (r *MapRegistry) List() []interfaces.Scenario {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]interfaces.Scenario, 0, len(r.scenarios))
	for _, sc := range r.scenarios {
		list = append(list, sc)
	}

	// Sort deterministically by ID
	sort.Slice(list, func(i, j int) bool {
		return list[i].ID() < list[j].ID()
	})

	return list
}

// Filter returns scenarios matching criteria in FilterOptions.
func (r *MapRegistry) Filter(filter interface{}) []interfaces.Scenario {
	opts, ok := filter.(FilterOptions)
	if !ok {
		return r.List() // fallback to full list
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []interfaces.Scenario
	for _, sc := range r.scenarios {
		// Filter by Priority if set (non-zero)
		if opts.Priority > 0 && sc.Priority() != opts.Priority {
			continue
		}

		// Filter by Capabilities if set
		if len(opts.Capabilities) > 0 {
			matchesAll := true
			scCaps := sc.Capabilities()
			for _, reqCap := range opts.Capabilities {
				found := false
				for _, c := range scCaps {
					if c == reqCap {
						found = true
						break
					}
				}
				if !found {
					matchesAll = false
					break
				}
			}
			if !matchesAll {
				continue
			}
		}

		result = append(result, sc)
	}

	// Sort deterministically
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID() < result[j].ID()
	})

	return result
}
