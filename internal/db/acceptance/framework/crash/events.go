package crash

import (
	"context"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/eventbus"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// Event payloads are typed structs (no interface{} inside this package).

// PointRegisteredPayload is published when a crash point is registered via Manager.
type PointRegisteredPayload struct {
	PointID    string
	EngineHook string
	Category   Category
	Phase      Phase
}

// EvaluationPayload summarizes a ShouldCrash evaluation.
type EvaluationPayload struct {
	ScenarioID   string
	ExecutionID  string
	CrashPointID string
	EngineHook   string
	ShouldCrash  bool
	Reason       string
	Policy       PolicyKind
	DryRun       bool
	Duration     time.Duration
}

// HookExecutedPayload is published after CrashHook.Execute succeeds.
type HookExecutedPayload struct {
	HookID       string
	CrashPointID string
	EngineHook   string
	Duration     time.Duration
}

// PolicyRejectedPayload is published when a policy forbids crashing.
type PolicyRejectedPayload struct {
	CrashPointID string
	Policy       PolicyKind
	Reason       string
}

func publish(bus *eventbus.EventBus, ctx context.Context, et types.EventType, payload typedPayload) {
	if bus == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	bus.Publish(ctx, et, payload)
}

// typedPayload constrains publish helpers to known payload types.
type typedPayload interface {
	isCrashEventPayload()
}

func (PointRegisteredPayload) isCrashEventPayload() {}
func (EvaluationPayload) isCrashEventPayload()      {}
func (HookExecutedPayload) isCrashEventPayload()    {}
func (PolicyRejectedPayload) isCrashEventPayload()  {}
