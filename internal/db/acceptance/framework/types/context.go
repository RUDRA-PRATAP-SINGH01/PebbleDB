// Package types defines the core ATF domain structures.
package types

import (
	"context"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/interfaces"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
)

// ExecutionContext bundles the immutable run parameters, cancellation controls,
// and coordination channels for a scenario execution session.
type ExecutionContext struct {
	Ctx          context.Context
	Cancel       context.CancelFunc
	CampaignID   string
	ScenarioID   string
	ExecutionID  string
	Logger       *logging.Logger
	Config       Configuration
	WorkingDir   string
	Timeout      time.Duration
	Telemetry    interfaces.TelemetryEngine
	EventBus     interfaces.EventSubscriber // EventBus acts as subscriber/publisher wrapper
}

// Deadline returns the time when work done on behalf of this context should be stopped.
func (c *ExecutionContext) Deadline() (deadline time.Time, ok bool) {
	return c.Ctx.Deadline()
}

// Done returns a channel that's closed when work done on behalf of this context should be canceled.
func (c *ExecutionContext) Done() <-chan struct{} {
	return c.Ctx.Done()
}

// Err returns a non-nil error if Done is closed.
func (c *ExecutionContext) Err() error {
	return c.Ctx.Err()
}

// Value returns the value associated with key, or nil if none.
func (c *ExecutionContext) Value(key interface{}) interface{} {
	return c.Ctx.Value(key)
}
