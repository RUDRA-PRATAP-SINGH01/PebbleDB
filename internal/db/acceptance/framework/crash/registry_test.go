package crash

import (
	"strconv"
	"sync"
	"testing"
)

func TestRegistryRegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	p := CrashPoint{
		ID: "t1", Name: "T1", Category: CategoryFlush, Phase: PhaseFlushManifest,
		EngineHook: "hook_t1", Enabled: true, Severity: SeverityHigh,
	}
	if err := r.Register(p, NewEngineBridgeHook(p)); err != nil {
		t.Fatal(err)
	}
	got, ok := r.Lookup("t1")
	if !ok || got.EngineHook != "hook_t1" {
		t.Fatalf("lookup: %+v ok=%v", got, ok)
	}
	got, ok = r.LookupByEngineHook("hook_t1")
	if !ok || got.ID != "t1" {
		t.Fatal("lookup by hook failed")
	}
}

func TestRegistryDuplicateID(t *testing.T) {
	r := NewRegistry()
	p := CrashPoint{
		ID: "dup", Name: "D", Category: CategoryFlush, Phase: PhaseFlushSST,
		EngineHook: "h1", Enabled: true,
	}
	if err := r.Register(p, NewEngineBridgeHook(p)); err != nil {
		t.Fatal(err)
	}
	p2 := p
	p2.EngineHook = "h2"
	err := r.Register(p2, NewEngineBridgeHook(p2))
	if err == nil {
		t.Fatal("expected duplicate")
	}
	if e, ok := err.(*Error); !ok || e.Code != ErrDuplicatePoint {
		t.Fatalf("want ErrDuplicatePoint, got %v", err)
	}
}

func TestRegistryDuplicateEngineHook(t *testing.T) {
	r := NewRegistry()
	p1 := CrashPoint{
		ID: "a", Name: "A", Category: CategoryFlush, Phase: PhaseFlushSST,
		EngineHook: "same", Enabled: true,
	}
	p2 := CrashPoint{
		ID: "b", Name: "B", Category: CategoryFlush, Phase: PhaseFlushSST,
		EngineHook: "same", Enabled: true,
	}
	if err := r.Register(p1, NewEngineBridgeHook(p1)); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(p2, NewEngineBridgeHook(p2)); err == nil {
		t.Fatal("expected duplicate hook")
	}
}

func TestRegistryDependencies(t *testing.T) {
	r := NewRegistry()
	base := CrashPoint{
		ID: "base", Name: "Base", Category: CategoryFlush, Phase: PhaseFlushSST,
		EngineHook: "base_hook", Enabled: true,
	}
	dep := CrashPoint{
		ID: "child", Name: "Child", Category: CategoryFlush, Phase: PhaseFlushManifest,
		EngineHook: "child_hook", Enabled: true, Dependencies: []string{"base"},
	}
	if err := r.Register(dep, NewEngineBridgeHook(dep)); err != nil {
		t.Fatal(err)
	}
	if err := r.ValidateDependencies(); err == nil {
		t.Fatal("expected missing dependency")
	}
	if err := r.Register(base, NewEngineBridgeHook(base)); err != nil {
		t.Fatal(err)
	}
	if err := r.ValidateDependencies(); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryPhaseAndCategory(t *testing.T) {
	r, err := NewBuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	flush := r.LookupByCategory(CategoryFlush)
	if len(flush) < 4 {
		t.Fatalf("flush points=%d", len(flush))
	}
	phase := r.LookupByPhase(PhaseFlushManifest)
	if len(phase) != 1 || phase[0].EngineHook != EngineFlushAfterManifest {
		t.Fatalf("phase: %+v", phase)
	}
	caps := r.Capabilities()
	if len(caps) != r.Len() {
		t.Fatal("capabilities mismatch")
	}
}

func TestRegistryConcurrency(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	errCh := make(chan error, 64)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := CrashPoint{
				ID:         "id_" + strconv.Itoa(i),
				Name:       "n",
				Category:   CategoryGeneric,
				Phase:      PhaseFlushSST,
				EngineHook: "hook_" + strconv.Itoa(i),
				Enabled:    true,
			}
			if err := r.Register(p, NewEngineBridgeHook(p)); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
	if r.Len() != 32 {
		t.Fatalf("len=%d", r.Len())
	}
}
