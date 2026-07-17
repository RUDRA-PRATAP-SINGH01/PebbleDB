package verifier

import "testing"

// stubVerifier is a no-op module for ordering tests.
type stubVerifier struct{ name string }

func (s stubVerifier) Name() string { return s.name }
func (s stubVerifier) Verify(*VerificationContext) (*ModuleResult, error) {
	return emptyModuleResult(s.name), nil
}

func regWith(names ...string) *Registry {
	r := NewRegistry()
	for _, n := range names {
		if err := r.Register(stubVerifier{name: n}); err != nil {
			panic(err)
		}
	}
	return r
}

func names(order moduleOrder) []string {
	out := make([]string, len(order.modules))
	for i, m := range order.modules {
		out[i] = m.Name()
	}
	return out
}

func TestResolveModuleOrderEmptyDAG(t *testing.T) {
	reg := regWith("a", "b", "c")
	order, err := resolveModuleOrder(reg, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := names(order)
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want registration order %v", got, want)
		}
	}
}

func TestResolveModuleOrderRespectsDeps(t *testing.T) {
	reg := regWith("get", "meta", "scan")
	dag := map[string][]string{
		"get":  {"meta"},
		"scan": {"get"},
		"meta": nil,
	}
	order, err := resolveModuleOrder(reg, dag)
	if err != nil {
		t.Fatal(err)
	}
	pos := map[string]int{}
	for i, m := range order.modules {
		pos[m.Name()] = i
	}
	if !(pos["meta"] < pos["get"] && pos["get"] < pos["scan"]) {
		t.Fatalf("dependency order violated: %v", names(order))
	}
}

func TestResolveModuleOrderIgnoresUnknownNodes(t *testing.T) {
	reg := regWith("get", "meta")
	dag := map[string][]string{
		"get":   {"meta", "does_not_exist"},
		"ghost": {"meta"},
		"meta":  nil,
	}
	order, err := resolveModuleOrder(reg, dag)
	if err != nil {
		t.Fatalf("unknown nodes must be ignored, got err: %v", err)
	}
	if len(order.modules) != 2 {
		t.Fatalf("expected 2 modules, got %v", names(order))
	}
}

func TestResolveModuleOrderDetectsCycle(t *testing.T) {
	reg := regWith("a", "b")
	dag := map[string][]string{
		"a": {"b"},
		"b": {"a"},
	}
	if _, err := resolveModuleOrder(reg, dag); err == nil {
		t.Fatal("expected cycle detection error")
	}
}

func TestResolveModuleOrderSelfCycle(t *testing.T) {
	reg := regWith("a")
	dag := map[string][]string{"a": {"a"}}
	if _, err := resolveModuleOrder(reg, dag); err == nil {
		t.Fatal("expected self-dependency error")
	}
}

func TestFirstUnhealthyDep(t *testing.T) {
	unhealthy := map[string]bool{"x": true}
	if got := firstUnhealthyDep([]string{"a", "x", "b"}, unhealthy); got != "x" {
		t.Fatalf("got %q, want x", got)
	}
	if got := firstUnhealthyDep([]string{"a", "b"}, unhealthy); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
