package crash

import "fmt"

// CrashHook is an independently executable bridge between ATF decisions and
// engine crash injection. Hooks must not call os.Exit; the storage engine
// performs the process exit when PEBBLEDB_CRASH_AT matches.
type CrashHook interface {
	// ID returns the stable hook identifier (usually the CrashPoint ID).
	ID() string
	// Supports reports whether the hook can run under the given context.
	Supports(ctx *CrashContext) bool
	// Execute performs hook-side work (env preparation markers, validation).
	// It must be deterministic and free of process termination.
	Execute(ctx *CrashContext) error
}

// EngineBridgeHook bridges a registered CrashPoint to the PEBBLEDB_CRASH_AT
// environment contract used by db.maybeCrash.
type EngineBridgeHook struct {
	pointID          string
	engineHook       string
	requiresFlush    bool
	requiresManifest bool
	requiresWAL      bool
}

// NewEngineBridgeHook constructs a bridge hook for a crash point.
func NewEngineBridgeHook(point CrashPoint) *EngineBridgeHook {
	return &EngineBridgeHook{
		pointID:          point.ID,
		engineHook:       point.EngineHook,
		requiresFlush:    point.RequiresFlush,
		requiresManifest: point.RequiresManifest,
		requiresWAL:      point.RequiresWAL,
	}
}

// ID returns the crash point ID.
func (h *EngineBridgeHook) ID() string { return h.pointID }

// EngineHookValue returns the PEBBLEDB_CRASH_AT value.
func (h *EngineBridgeHook) EngineHookValue() string { return h.engineHook }

// Supports validates scenario/environment prerequisites for the engine hook.
func (h *EngineBridgeHook) Supports(ctx *CrashContext) bool {
	if ctx == nil {
		return false
	}
	if h.engineHook == "" {
		return false
	}
	env := ctx.Environment()
	if h.requiresFlush {
		if v, ok := env["PEBBLEDB_FORCE_FLUSH"]; ok && v == "0" {
			return false
		}
	}
	// Manifest/WAL requirements are informational for this milestone; the
	// engine path always involves those subsystems when flush/compact runs.
	_ = h.requiresManifest
	_ = h.requiresWAL
	return true
}

// Execute records that the bridge is armed. Actual process exit remains in db.
func (h *EngineBridgeHook) Execute(ctx *CrashContext) error {
	if ctx == nil {
		return newError(ErrHookExecute, "nil crash context", nil)
	}
	if !h.Supports(ctx) {
		return newError(ErrHookRejected, fmt.Sprintf("hook %s unsupported in context", h.pointID), nil)
	}
	return nil
}

// NoopHook is a test double that never fails Supports/Execute.
type NoopHook struct {
	HookID string
}

// ID returns the hook identifier.
func (h NoopHook) ID() string {
	if h.HookID == "" {
		return "noop"
	}
	return h.HookID
}

// Supports always returns true.
func (h NoopHook) Supports(ctx *CrashContext) bool { return ctx != nil }

// Execute is a no-op success.
func (h NoopHook) Execute(ctx *CrashContext) error {
	if ctx == nil {
		return newError(ErrHookExecute, "nil crash context", nil)
	}
	return nil
}
