package crash

import (
	"context"
	"sync"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/eventbus"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/telemetry"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// Decision is the deterministic outcome of a crash evaluation.
type Decision struct {
	ShouldCrash  bool
	CrashPointID string
	EngineHook   string
	Reason       string
	Policy       PolicyKind
	DryRun       bool
	Skipped      bool
	HookExecuted bool
}

// EnvKeyCrashAt is the environment variable consumed by db.maybeCrash.
const EnvKeyCrashAt = "PEBBLEDB_CRASH_AT"

// Manager owns crash configuration, policy evaluation, hook invocation, events,
// and telemetry. The execution engine asks ShouldCrash and applies ChildEnv.
type Manager struct {
	mu        sync.Mutex
	registry  *Registry
	config    Config
	logger    *logging.Logger
	eventBus  *eventbus.EventBus
	telemetry *telemetry.TelemetryStore
	evaluator *policyEvaluator
}

// NewManager constructs a CrashManager with dependency injection.
func NewManager(
	registry *Registry,
	logger *logging.Logger,
	eb *eventbus.EventBus,
	ts *telemetry.TelemetryStore,
) *Manager {
	return &Manager{
		registry:  registry,
		logger:    logger,
		eventBus:  eb,
		telemetry: ts,
		evaluator: newPolicyEvaluator(),
		config:    DefaultConfig(),
	}
}

// Configure validates and installs manager configuration.
func (m *Manager) Configure(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if cfg.Enabled && cfg.CrashPointID != "" && m.registry != nil {
		if _, _, err := m.registry.Resolve(cfg.CrashPointID); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.config = cfg
	m.mu.Unlock()
	return nil
}

// Config returns a copy of the active configuration.
func (m *Manager) Config() Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.config
}

// Registry returns the underlying crash registry.
func (m *Manager) Registry() *Registry { return m.registry }

// RegisterPoint registers a crash point, publishes an event, and updates telemetry.
func (m *Manager) RegisterPoint(ctx context.Context, point CrashPoint, hook CrashHook) error {
	if m.registry == nil {
		return newError(ErrInvalidConfig, "nil registry", nil)
	}
	if err := m.registry.Register(point, hook); err != nil {
		return err
	}
	publish(m.eventBus, ctx, types.EventCrashPointRegistered, PointRegisteredPayload{
		PointID:    point.ID,
		EngineHook: point.EngineHook,
		Category:   point.Category,
		Phase:      point.Phase,
	})
	m.metric("", "crash_points_registered", float64(m.registry.Len()))
	return nil
}

// ShouldCrash resolves the configured (or context) crash point, evaluates policy,
// optionally executes the hook, and returns a deterministic Decision.
func (m *Manager) ShouldCrash(ctx context.Context, cctx *CrashContext) (Decision, error) {
	start := time.Now()
	m.mu.Lock()
	cfg := m.config
	m.mu.Unlock()

	if cctx == nil {
		return Decision{}, newError(ErrInvalidConfig, "nil crash context", nil)
	}
	if err := cfg.Validate(); err != nil {
		return Decision{}, err
	}

	scenarioID := cctx.ScenarioID()
	publish(m.eventBus, ctx, types.EventCrashEvaluationStarted, EvaluationPayload{
		ScenarioID:   scenarioID,
		ExecutionID:  cctx.ExecutionID(),
		CrashPointID: cfg.CrashPointID,
		Policy:       cfg.Policy.Kind,
		DryRun:       cfg.DryRun,
	})

	finish := func(d Decision, err error) (Decision, error) {
		publish(m.eventBus, ctx, types.EventCrashEvaluationFinished, EvaluationPayload{
			ScenarioID:   scenarioID,
			ExecutionID:  cctx.ExecutionID(),
			CrashPointID: d.CrashPointID,
			EngineHook:   d.EngineHook,
			ShouldCrash:  d.ShouldCrash,
			Reason:       d.Reason,
			Policy:       d.Policy,
			DryRun:       d.DryRun,
			Duration:     time.Since(start),
		})
		m.duration(scenarioID, "crash_evaluation", time.Since(start))
		m.metric(scenarioID, "crash_policy_evaluations", 1)
		return d, err
	}

	if cfg.ValidationOnly {
		if cfg.CrashPointID != "" {
			if _, _, err := m.registry.Resolve(cfg.CrashPointID); err != nil {
				return finish(Decision{Reason: err.Error(), Policy: cfg.Policy.Kind}, err)
			}
		}
		return finish(Decision{
			ShouldCrash:  false,
			CrashPointID: cfg.CrashPointID,
			Reason:       "validation_only",
			Policy:       cfg.Policy.Kind,
			Skipped:      true,
		}, nil)
	}

	if !cfg.Enabled {
		d := Decision{ShouldCrash: false, Reason: "manager disabled", Policy: cfg.Policy.Kind, Skipped: true}
		m.metric(scenarioID, "crash_points_skipped", 1)
		publish(m.eventBus, ctx, types.EventCrashSkipped, EvaluationPayload{
			ScenarioID: scenarioID, CrashPointID: cfg.CrashPointID, Reason: d.Reason, Policy: d.Policy,
		})
		return finish(d, nil)
	}

	pointID := cfg.CrashPointID
	if pointID == "" {
		pointID = cctx.ScenarioCrashID()
	}
	if pointID == "" {
		d := Decision{ShouldCrash: false, Reason: "no crash point requested", Policy: cfg.Policy.Kind, Skipped: true}
		m.metric(scenarioID, "crash_points_skipped", 1)
		publish(m.eventBus, ctx, types.EventCrashSkipped, EvaluationPayload{
			ScenarioID: scenarioID, Reason: d.Reason, Policy: d.Policy,
		})
		return finish(d, nil)
	}

	point, hook, err := m.registry.Resolve(pointID)
	if err != nil {
		return finish(Decision{CrashPointID: pointID, Reason: err.Error(), Policy: cfg.Policy.Kind}, err)
	}
	if !point.Enabled {
		d := Decision{
			ShouldCrash: false, CrashPointID: point.ID, EngineHook: point.EngineHook,
			Reason: "crash point disabled", Policy: cfg.Policy.Kind, Skipped: true,
		}
		publish(m.eventBus, ctx, types.EventCrashSkipped, EvaluationPayload{
			ScenarioID: scenarioID, CrashPointID: point.ID, EngineHook: point.EngineHook, Reason: d.Reason, Policy: d.Policy,
		})
		m.metric(scenarioID, "crash_points_skipped", 1)
		return finish(d, nil)
	}

	// Rebuild context with resolved point for hook Supports/Execute.
	evalCtx := NewCrashContext(CrashContextParams{
		ExecutionID:     cctx.ExecutionID(),
		ScenarioID:      cctx.ScenarioID(),
		DatabasePath:    cctx.DatabasePath(),
		Logger:          cctx.Logger(),
		Config:          cctx.Config(),
		Telemetry:       cctx.Telemetry(),
		EventBus:        cctx.EventBus(),
		ExecutionState:  cctx.ExecutionState(),
		CrashPoint:      point,
		RandomSeed:      cfg.effectivePolicy().Seed,
		Environment:     cctx.Environment(),
		Child:           cctx.Child(),
		WorkingDir:      cctx.WorkingDir(),
		ScenarioCrashID: cctx.ScenarioCrashID(),
	})

	policy := cfg.effectivePolicy()
	pr, err := m.evaluator.evaluate(point.ID, point.EngineHook, policy, cctx.ScenarioCrashID())
	if err != nil {
		if e, ok := err.(*Error); ok && e.Code == ErrPolicyUnsupported {
			publish(m.eventBus, ctx, types.EventCrashPolicyRejected, PolicyRejectedPayload{
				CrashPointID: point.ID, Policy: policy.Kind, Reason: e.Message,
			})
		}
		return finish(Decision{CrashPointID: point.ID, EngineHook: point.EngineHook, Policy: policy.Kind, Reason: err.Error()}, err)
	}

	if !pr.crash {
		d := Decision{
			ShouldCrash: false, CrashPointID: point.ID, EngineHook: point.EngineHook,
			Reason: pr.reason, Policy: policy.Kind, Skipped: true, DryRun: cfg.DryRun,
		}
		publish(m.eventBus, ctx, types.EventCrashPolicyRejected, PolicyRejectedPayload{
			CrashPointID: point.ID, Policy: policy.Kind, Reason: pr.reason,
		})
		publish(m.eventBus, ctx, types.EventCrashSkipped, EvaluationPayload{
			ScenarioID: scenarioID, CrashPointID: point.ID, EngineHook: point.EngineHook,
			Reason: pr.reason, Policy: policy.Kind, DryRun: cfg.DryRun,
		})
		m.metric(scenarioID, "crash_points_skipped", 1)
		return finish(d, nil)
	}

	if !hook.Supports(evalCtx) {
		d := Decision{
			ShouldCrash: false, CrashPointID: point.ID, EngineHook: point.EngineHook,
			Reason: "hook does not support context", Policy: policy.Kind, Skipped: true,
		}
		publish(m.eventBus, ctx, types.EventCrashSkipped, EvaluationPayload{
			ScenarioID: scenarioID, CrashPointID: point.ID, Reason: d.Reason, Policy: d.Policy,
		})
		m.metric(scenarioID, "crash_points_skipped", 1)
		return finish(d, nil)
	}

	hookStart := time.Now()
	if err := hook.Execute(evalCtx); err != nil {
		return finish(Decision{
			CrashPointID: point.ID, EngineHook: point.EngineHook, Policy: policy.Kind, Reason: err.Error(),
		}, newError(ErrHookExecute, "hook execute failed", err))
	}
	hookDur := time.Since(hookStart)
	m.duration(scenarioID, "crash_hook_execute", hookDur)
	publish(m.eventBus, ctx, types.EventCrashHookExecuted, HookExecutedPayload{
		HookID: hook.ID(), CrashPointID: point.ID, EngineHook: point.EngineHook, Duration: hookDur,
	})

	d := Decision{
		ShouldCrash:  true,
		CrashPointID: point.ID,
		EngineHook:   point.EngineHook,
		Reason:       pr.reason,
		Policy:       policy.Kind,
		DryRun:       cfg.DryRun,
		HookExecuted: true,
	}
	if cfg.DryRun {
		d.ShouldCrash = false
		d.Skipped = true
		d.Reason = pr.reason + "; dry_run"
		publish(m.eventBus, ctx, types.EventCrashSkipped, EvaluationPayload{
			ScenarioID: scenarioID, CrashPointID: point.ID, EngineHook: point.EngineHook,
			Reason: d.Reason, Policy: d.Policy, DryRun: true,
		})
		m.metric(scenarioID, "crash_points_skipped", 1)
		return finish(d, nil)
	}

	publish(m.eventBus, ctx, types.EventCrashTriggered, EvaluationPayload{
		ScenarioID: scenarioID, ExecutionID: cctx.ExecutionID(),
		CrashPointID: point.ID, EngineHook: point.EngineHook,
		ShouldCrash: true, Reason: d.Reason, Policy: d.Policy,
	})
	m.metric(scenarioID, "crash_points_executed", 1)
	if m.logger != nil {
		m.logger.Info("ATF crash triggered point=%s hook=%s reason=%s", point.ID, point.EngineHook, d.Reason)
	}
	return finish(d, nil)
}

// ChildEnv returns environment assignments for the ATF child process based on a Decision.
// When ShouldCrash is false, PEBBLEDB_CRASH_AT is cleared (empty) so the child will not crash.
func (m *Manager) ChildEnv(decision Decision) map[string]string {
	out := make(map[string]string, 1)
	if decision.ShouldCrash && decision.EngineHook != "" {
		out[EnvKeyCrashAt] = decision.EngineHook
		return out
	}
	out[EnvKeyCrashAt] = ""
	return out
}

// PrepareForScenario is a convenience for the execution engine: configure
// scenario-controlled Always policy for the given crash point (ID or EngineHook).
func (m *Manager) PrepareForScenario(crashPoint string) error {
	if crashPoint == "" {
		return m.Configure(Config{
			Enabled: false,
			Policy:  Policy{Kind: PolicyNever},
		})
	}
	return m.Configure(Config{
		CrashPointID: crashPoint,
		Policy:       Policy{Kind: PolicyAlways},
		Enabled:      true,
	})
}

// EvaluateForScenario builds a context and returns a Decision for the scenario crash point.
func (m *Manager) EvaluateForScenario(
	ctx context.Context,
	exec types.ExecutionSession,
	scenarioID string,
	scenarioCrash string,
	baseEnv map[string]string,
) (Decision, error) {
	if err := m.PrepareForScenario(scenarioCrash); err != nil {
		return Decision{}, err
	}
	cctx := NewCrashContext(CrashContextParams{
		ExecutionID:     exec.SessionID,
		ScenarioID:      scenarioID,
		DatabasePath:    exec.TempDir,
		Logger:          m.logger,
		Telemetry:       m.telemetry,
		EventBus:        m.eventBus,
		ExecutionState:  exec.StateVal,
		RandomSeed:      m.Config().RandomSeed,
		Environment:     baseEnv,
		WorkingDir:      exec.TempDir,
		ScenarioCrashID: scenarioCrash,
		Child:           ChildProcessInfo{IsChild: false},
	})
	return m.ShouldCrash(ctx, cctx)
}

func (m *Manager) duration(scenarioID, stage string, d time.Duration) {
	if m.telemetry == nil {
		return
	}
	m.telemetry.RecordDuration(scenarioID, stage, d)
}

func (m *Manager) metric(scenarioID, name string, value float64) {
	if m.telemetry == nil {
		return
	}
	m.telemetry.RecordMetric(scenarioID, name, value)
}
