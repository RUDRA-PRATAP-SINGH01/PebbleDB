package crash

import "testing"

func TestEngineBridgeHookSupportsFlushRequirement(t *testing.T) {
	p := CrashPoint{
		ID: "f", Name: "F", Category: CategoryFlush, Phase: PhaseFlushSST,
		EngineHook: EngineFlushAfterManifest, Enabled: true, RequiresFlush: true,
	}
	h := NewEngineBridgeHook(p)
	ctx := NewCrashContext(CrashContextParams{
		Environment: map[string]string{"PEBBLEDB_FORCE_FLUSH": "0"},
		CrashPoint:  p,
	})
	if h.Supports(ctx) {
		t.Fatal("should reject FORCE_FLUSH=0")
	}
	ctx = NewCrashContext(CrashContextParams{
		Environment: map[string]string{"PEBBLEDB_FORCE_FLUSH": "1"},
		CrashPoint:  p,
	})
	if !h.Supports(ctx) {
		t.Fatal("should support flush=1")
	}
	if err := h.Execute(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestNoopHook(t *testing.T) {
	h := NoopHook{HookID: "n"}
	ctx := NewCrashContext(CrashContextParams{})
	if !h.Supports(ctx) || h.Execute(ctx) != nil {
		t.Fatal("noop failed")
	}
}
