package verifier

import "fmt"

// moduleOrder is a deterministic verifier execution plan derived from a scenario
// verification DAG. When the DAG is empty every registered module runs in
// registration order. Otherwise modules are topologically ordered so that a
// module never runs before its dependencies, and any registered module absent
// from the DAG is appended afterwards in registration order.
type moduleOrder struct {
	modules []Verifier
	// deps maps a module name to its registered dependency names.
	deps map[string][]string
}

// resolveModuleOrder builds an execution plan from the registry and DAG.
// Unknown module names in the DAG are ignored (advisory graph). A cycle among
// registered modules is a hard error.
func resolveModuleOrder(reg *Registry, dag map[string][]string) (moduleOrder, error) {
	all := reg.All()
	registered := make(map[string]Verifier, len(all))
	regOrder := make([]string, 0, len(all))
	for _, v := range all {
		registered[v.Name()] = v
		regOrder = append(regOrder, v.Name())
	}

	if len(dag) == 0 {
		return moduleOrder{modules: all, deps: map[string][]string{}}, nil
	}

	// Keep only edges between registered modules.
	deps := make(map[string][]string, len(dag))
	indegree := make(map[string]int, len(registered))
	adj := make(map[string][]string, len(registered))
	for name := range registered {
		indegree[name] = 0
	}
	for node, rawDeps := range dag {
		if _, ok := registered[node]; !ok {
			continue
		}
		for _, dep := range rawDeps {
			if _, ok := registered[dep]; !ok {
				continue
			}
			if dep == node {
				return moduleOrder{}, fmt.Errorf("verifier dag: module %q depends on itself", node)
			}
			deps[node] = append(deps[node], dep)
			adj[dep] = append(adj[dep], node)
			indegree[node]++
		}
	}

	// Kahn's algorithm over registration order for deterministic output.
	queue := make([]string, 0, len(registered))
	for _, name := range regOrder {
		if indegree[name] == 0 {
			queue = append(queue, name)
		}
	}
	ordered := make([]Verifier, 0, len(registered))
	seen := make(map[string]bool, len(registered))
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if seen[name] {
			continue
		}
		seen[name] = true
		ordered = append(ordered, registered[name])
		// Preserve registration order among newly-freed successors.
		var freed []string
		for _, succ := range adj[name] {
			indegree[succ]--
			if indegree[succ] == 0 {
				freed = append(freed, succ)
			}
		}
		freed = sortByRegOrder(freed, regOrder)
		queue = append(queue, freed...)
	}

	if len(ordered) != len(registered) {
		return moduleOrder{}, fmt.Errorf("verifier dag: cycle detected among %d modules", len(registered)-len(ordered))
	}
	return moduleOrder{modules: ordered, deps: deps}, nil
}

// firstUnhealthyDep returns the first dependency present in the unhealthy set,
// or "" when all dependencies are healthy.
func firstUnhealthyDep(deps []string, unhealthy map[string]bool) string {
	for _, d := range deps {
		if unhealthy[d] {
			return d
		}
	}
	return ""
}

func sortByRegOrder(names, regOrder []string) []string {
	rank := make(map[string]int, len(regOrder))
	for i, n := range regOrder {
		rank[n] = i
	}
	// simple insertion sort (small slices)
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && rank[names[j-1]] > rank[names[j]]; j-- {
			names[j-1], names[j] = names[j], names[j-1]
		}
	}
	return names
}
