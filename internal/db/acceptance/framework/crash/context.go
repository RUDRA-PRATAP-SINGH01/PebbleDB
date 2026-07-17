package crash

import (
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/eventbus"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/telemetry"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// ChildProcessInfo describes the ATF child process envelope.
type ChildProcessInfo struct {
	// Binary is the executable path (often os.Args[0] in tests).
	Binary string
	// PID is set when known; zero if not yet spawned.
	PID int
	// IsChild is true when running inside the ATF child process.
	IsChild bool
}

// CrashContext is the immutable input bundle for crash evaluation and hooks.
type CrashContext struct {
	executionID    string
	scenarioID     string
	databasePath   string
	logger         *logging.Logger
	config         types.Configuration
	telemetry      *telemetry.TelemetryStore
	eventBus       *eventbus.EventBus
	executionState types.State
	crashPoint     CrashPoint
	randomSeed     int64
	environment    map[string]string
	child          ChildProcessInfo
	workingDir     string
	// scenarioCrashID is the crash point requested by the scenario definition.
	scenarioCrashID string
}

// CrashContextParams constructs a CrashContext.
type CrashContextParams struct {
	ExecutionID     string
	ScenarioID      string
	DatabasePath    string
	Logger          *logging.Logger
	Config          types.Configuration
	Telemetry       *telemetry.TelemetryStore
	EventBus        *eventbus.EventBus
	ExecutionState  types.State
	CrashPoint      CrashPoint
	RandomSeed      int64
	Environment     map[string]string
	Child           ChildProcessInfo
	WorkingDir      string
	ScenarioCrashID string
}

// NewCrashContext builds an immutable CrashContext (deep-copies maps).
func NewCrashContext(p CrashContextParams) *CrashContext {
	env := make(map[string]string, len(p.Environment))
	for k, v := range p.Environment {
		env[k] = v
	}
	return &CrashContext{
		executionID:     p.ExecutionID,
		scenarioID:      p.ScenarioID,
		databasePath:    p.DatabasePath,
		logger:          p.Logger,
		config:          p.Config,
		telemetry:       p.Telemetry,
		eventBus:        p.EventBus,
		executionState:  p.ExecutionState,
		crashPoint:      p.CrashPoint.Clone(),
		randomSeed:      p.RandomSeed,
		environment:     env,
		child:           p.Child,
		workingDir:      p.WorkingDir,
		scenarioCrashID: p.ScenarioCrashID,
	}
}

// ExecutionID returns the execution session identifier.
func (c *CrashContext) ExecutionID() string { return c.executionID }

// ScenarioID returns the scenario identifier.
func (c *CrashContext) ScenarioID() string { return c.scenarioID }

// DatabasePath returns the database directory.
func (c *CrashContext) DatabasePath() string { return c.databasePath }

// Logger returns the injected logger.
func (c *CrashContext) Logger() *logging.Logger { return c.logger }

// Config returns framework configuration.
func (c *CrashContext) Config() types.Configuration { return c.config }

// Telemetry returns the telemetry store.
func (c *CrashContext) Telemetry() *telemetry.TelemetryStore { return c.telemetry }

// EventBus returns the event bus.
func (c *CrashContext) EventBus() *eventbus.EventBus { return c.eventBus }

// ExecutionState returns the current execution lifecycle state.
func (c *CrashContext) ExecutionState() types.State { return c.executionState }

// CrashPoint returns the crash point under evaluation.
func (c *CrashContext) CrashPoint() CrashPoint { return c.crashPoint.Clone() }

// RandomSeed returns the evaluation seed.
func (c *CrashContext) RandomSeed() int64 { return c.randomSeed }

// Environment returns a copy of environment key/values.
func (c *CrashContext) Environment() map[string]string {
	out := make(map[string]string, len(c.environment))
	for k, v := range c.environment {
		out[k] = v
	}
	return out
}

// Child returns child process information.
func (c *CrashContext) Child() ChildProcessInfo { return c.child }

// WorkingDir returns the working directory for the execution.
func (c *CrashContext) WorkingDir() string { return c.workingDir }

// ScenarioCrashID returns the crash point requested by the scenario.
func (c *CrashContext) ScenarioCrashID() string { return c.scenarioCrashID }
