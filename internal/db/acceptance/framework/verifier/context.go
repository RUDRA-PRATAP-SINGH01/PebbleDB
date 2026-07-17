package verifier

import (
	"context"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/dataset"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/eventbus"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/telemetry"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// VerificationContext is the immutable input bundle for verifier modules.
// Identity and dependency fields are fixed after construction; Report is the
// shared aggregate updated only by VerificationEngine.
type VerificationContext struct {
	ctx          context.Context
	executionID  string
	scenarioID   string
	expected     *dataset.MapExpectedState
	databasePath string
	database     *db.DB
	logger       *logging.Logger
	telemetry    *telemetry.TelemetryStore
	config       types.Configuration
	eventBus     *eventbus.EventBus
	registry     *Registry
	report       *VerificationReport
}

// Context returns the cancellation context.
func (c *VerificationContext) Context() context.Context { return c.ctx }

// ExecutionID returns the execution session identifier.
func (c *VerificationContext) ExecutionID() string { return c.executionID }

// ScenarioID returns the scenario identifier.
func (c *VerificationContext) ScenarioID() string { return c.scenarioID }

// Expected returns the loaded oracle state.
func (c *VerificationContext) Expected() *dataset.MapExpectedState { return c.expected }

// DatabasePath returns the recovered database directory.
func (c *VerificationContext) DatabasePath() string { return c.databasePath }

// Database returns the open recovered DB handle.
func (c *VerificationContext) Database() *db.DB { return c.database }

// Logger returns the injected logger.
func (c *VerificationContext) Logger() *logging.Logger { return c.logger }

// Telemetry returns the telemetry store.
func (c *VerificationContext) Telemetry() *telemetry.TelemetryStore { return c.telemetry }

// Config returns framework configuration used for open options.
func (c *VerificationContext) Config() types.Configuration { return c.config }

// EventBus returns the event bus (may be nil in unit tests).
func (c *VerificationContext) EventBus() *eventbus.EventBus { return c.eventBus }

// Registry returns the verifier registry.
func (c *VerificationContext) Registry() *Registry { return c.registry }

// Report returns the in-progress verification report.
func (c *VerificationContext) Report() *VerificationReport { return c.report }

// Err returns ctx.Err().
func (c *VerificationContext) Err() error {
	if c == nil || c.ctx == nil {
		return nil
	}
	return c.ctx.Err()
}
