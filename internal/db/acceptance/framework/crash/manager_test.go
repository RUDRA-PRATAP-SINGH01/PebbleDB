package crash

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/eventbus"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/interfaces"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/telemetry"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

type eventCollector struct {
	mu   sync.Mutex
	seen []types.EventType
}

func (e *eventCollector) Name() string { return "crash_test_collector" }
func (e *eventCollector) OnEvent(ctx context.Context, event any) error {
	ev, ok := event.(types.Event)
	if !ok {
		return nil
	}
	e.mu.Lock()
	e.seen = append(e.seen, ev.Type)
	e.mu.Unlock()
	return nil
}

func (e *eventCollector) has(t types.EventType) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, x := range e.seen {
		if x == t {
			return true
		}
	}
	return false
}

func (e *eventCollector) waitHas(t *testing.T, et types.EventType) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if e.has(et) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for event %v; seen=%v", et, e.seen)
}

var _ interfaces.EventSubscriber = (*eventCollector)(nil)

func testManager(t *testing.T) (*Manager, *eventCollector, *telemetry.TelemetryStore) {
	t.Helper()
	reg, err := NewBuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	logger := logging.NewLogger(os.Stderr, logging.LevelError)
	bus := eventbus.NewEventBus(logger)
	if err := bus.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bus.Stop() })
	col := &eventCollector{}
	if err := bus.Subscribe(col); err != nil {
		t.Fatal(err)
	}
	ts := telemetry.NewTelemetryStore()
	return NewManager(reg, logger, bus, ts), col, ts
}

func TestManagerShouldCrashAlways(t *testing.T) {
	m, col, _ := testManager(t)
	if err := m.Configure(Config{
		CrashPointID: EngineFlushAfterManifest,
		Policy:       Policy{Kind: PolicyAlways},
		Enabled:      true,
	}); err != nil {
		t.Fatal(err)
	}
	ctx := NewCrashContext(CrashContextParams{
		ScenarioID: "s", ExecutionID: "e",
		Environment:     map[string]string{"PEBBLEDB_FORCE_FLUSH": "1"},
		ScenarioCrashID: EngineFlushAfterManifest,
	})
	d, err := m.ShouldCrash(context.Background(), ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !d.ShouldCrash || d.EngineHook != EngineFlushAfterManifest || !d.HookExecuted {
		t.Fatalf("decision=%+v", d)
	}
	env := m.ChildEnv(d)
	if env[EnvKeyCrashAt] != EngineFlushAfterManifest {
		t.Fatalf("env=%v", env)
	}
	col.waitHas(t, types.EventCrashTriggered)
	col.waitHas(t, types.EventCrashHookExecuted)
}

func TestManagerDryRun(t *testing.T) {
	m, col, _ := testManager(t)
	_ = m.Configure(Config{
		CrashPointID: EngineFlushAfterWalState,
		Policy:       Policy{Kind: PolicyAlways},
		Enabled:      true,
		DryRun:       true,
	})
	d, err := m.ShouldCrash(context.Background(), NewCrashContext(CrashContextParams{
		Environment: map[string]string{"PEBBLEDB_FORCE_FLUSH": "1"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if d.ShouldCrash || !d.HookExecuted || !d.Skipped {
		t.Fatalf("dry run decision=%+v", d)
	}
	col.waitHas(t, types.EventCrashSkipped)
}

func TestManagerPolicyNever(t *testing.T) {
	m, col, _ := testManager(t)
	_ = m.Configure(Config{
		CrashPointID: EngineFlushAfterManifest,
		Policy:       Policy{Kind: PolicyNever},
		Enabled:      true,
	})
	d, err := m.ShouldCrash(context.Background(), NewCrashContext(CrashContextParams{
		Environment: map[string]string{"PEBBLEDB_FORCE_FLUSH": "1"},
	}))
	if err != nil || d.ShouldCrash {
		t.Fatalf("d=%+v err=%v", d, err)
	}
	col.waitHas(t, types.EventCrashPolicyRejected)
}

func TestManagerValidationOnly(t *testing.T) {
	m, _, _ := testManager(t)
	if err := m.Configure(Config{
		CrashPointID:   EngineFlushAfterManifest,
		Policy:         Policy{Kind: PolicyAlways},
		Enabled:        true,
		ValidationOnly: true,
	}); err != nil {
		t.Fatal(err)
	}
	d, err := m.ShouldCrash(context.Background(), NewCrashContext(CrashContextParams{}))
	if err != nil || d.ShouldCrash || !d.Skipped {
		t.Fatalf("d=%+v err=%v", d, err)
	}
}

func TestManagerInvalidPoint(t *testing.T) {
	m, _, _ := testManager(t)
	err := m.Configure(Config{
		CrashPointID: "does.not.exist",
		Policy:       Policy{Kind: PolicyAlways},
		Enabled:      true,
	})
	if err == nil {
		t.Fatal("expected configure failure")
	}
}

func TestManagerEvaluateForScenario(t *testing.T) {
	m, _, ts := testManager(t)
	d, err := m.EvaluateForScenario(
		context.Background(),
		types.ExecutionSession{SessionID: "sess", TempDir: t.TempDir(), StateVal: types.StateSubprocessWriting},
		"EXS-010",
		EngineFlushAfterManifest,
		map[string]string{"PEBBLEDB_FORCE_FLUSH": "1"},
	)
	if err != nil || !d.ShouldCrash {
		t.Fatalf("d=%+v err=%v", d, err)
	}
	dump := ts.Dump()
	if dump == nil {
		t.Fatal("expected telemetry")
	}
}

func TestManagerRegisterPointEvent(t *testing.T) {
	m, col, _ := testManager(t)
	p := CrashPoint{
		ID: "custom.point", Name: "Custom", Category: CategoryGeneric, Phase: PhaseFlushSST,
		EngineHook: "custom_engine_hook", Enabled: true,
	}
	if err := m.RegisterPoint(context.Background(), p, NewEngineBridgeHook(p)); err != nil {
		t.Fatal(err)
	}
	col.waitHas(t, types.EventCrashPointRegistered)
}

func TestManagerDisabled(t *testing.T) {
	m, _, _ := testManager(t)
	_ = m.Configure(Config{Enabled: false, Policy: Policy{Kind: PolicyNever}})
	d, err := m.ShouldCrash(context.Background(), NewCrashContext(CrashContextParams{}))
	if err != nil || d.ShouldCrash || !d.Skipped {
		t.Fatalf("d=%+v err=%v", d, err)
	}
}

func TestBuiltinRegistryAlignsWithEngineHooks(t *testing.T) {
	r, err := NewBuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		EngineFlushAfterSSTClose, EngineFlushAfterManifest, EngineFlushAfterWalState,
		EngineFlushAfterWalTruncate, EngineCompactAfterMergeClose, EngineCompactAfterManifest,
		EngineCompactAfterSSTables, EngineCompactAfterDeleteOld,
	}
	if r.Len() != len(want) {
		t.Fatalf("len=%d", r.Len())
	}
	for _, h := range want {
		if _, ok := r.LookupByEngineHook(h); !ok {
			t.Fatalf("missing hook %s", h)
		}
	}
}
