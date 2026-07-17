// Package session provides session trackers, lifecycle validation engines,
// and state transition checkers to enforce immutable execution boundaries.
//
// Dependency Rules:
// - Imports: interfaces, types, errors.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/errors"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// AllowedTransitions maps each state to the states it can legally transition into.
var AllowedTransitions = map[types.State][]types.State{
	types.StateInit: {
		types.StateCampaignRunning,
	},
	types.StateCampaignRunning: {
		types.StateScenarioRunning,
		types.StateCampaignCompleted,
	},
	types.StateScenarioRunning: {
		types.StateExecutionPrepared,
		types.StateScenarioCompleted,
		types.StateScenarioFailed,
	},
	types.StateExecutionPrepared: {
		types.StateSubprocessWriting,
		types.StateScenarioFailed,
	},
	types.StateSubprocessWriting: {
		types.StateSubprocessCrashed,
		types.StateScenarioFailed,
	},
	types.StateSubprocessCrashed: {
		types.StateDirectorySnapshoted,
		types.StateScenarioFailed,
	},
	types.StateDirectorySnapshoted: {
		types.StateRecoveryAttempted,
		types.StateScenarioFailed,
	},
	types.StateRecoveryAttempted: {
		types.StateVerificationRunning,
		types.StateScenarioFailed,
	},
	types.StateVerificationRunning: {
		types.StateEvidenceCollected,
		types.StateScenarioFailed,
	},
	types.StateEvidenceCollected: {
		types.StateScenarioCompleted,
		types.StateScenarioFailed,
	},
	types.StateScenarioCompleted: {
		types.StateScenarioRunning,
		types.StateCampaignCompleted,
	},
	types.StateScenarioFailed: {
		types.StateScenarioRunning,
		types.StateCampaignCompleted,
	},
}

// SessionTracker coordinates thread-safe state transition checking.
type SessionTracker struct {
	mu        sync.RWMutex
	sessionID string
	state     types.State
	createdAt time.Time
}

// NewSessionTracker allocates a new SessionTracker with a generated UUID.
func NewSessionTracker(initialState types.State) *SessionTracker {
	return &SessionTracker{
		sessionID: generateUUID(),
		state:     initialState,
		createdAt: time.Now(),
	}
}

// ID returns the unique session UUID.
func (s *SessionTracker) ID() string {
	return s.sessionID
}

// State returns the current session State.
func (s *SessionTracker) State() types.State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// CreatedAt returns the creation timestamp.
func (s *SessionTracker) CreatedAt() time.Time {
	return s.createdAt
}

// Transition moves the session to newState if the transition is allowed.
func (s *SessionTracker) Transition(newState types.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	allowed, ok := AllowedTransitions[s.state]
	if !ok {
		return errors.NewStateError(fmt.Sprintf("stuck state: %s", s.state), nil)
	}

	valid := false
	for _, a := range allowed {
		if a == newState {
			valid = true
			break
		}
	}

	if !valid {
		return errors.NewStateError(fmt.Sprintf("forbidden transition: %s -> %s", s.state, newState), nil)
	}

	s.state = newState
	return nil
}

// CampaignTracker wraps CampaignSession lifecycle operations.
type CampaignTracker struct {
	*SessionTracker
	metadata  types.Metadata
	scenarios []types.ScenarioResult
	mu        sync.Mutex
}

// NewCampaignTracker creates a new CampaignTracker.
func NewCampaignTracker(meta types.Metadata) *CampaignTracker {
	return &CampaignTracker{
		SessionTracker: NewSessionTracker(types.StateInit),
		metadata:       meta,
		scenarios:      make([]types.ScenarioResult, 0),
	}
}

// AddScenarioResult registers a completed scenario run outcome.
func (c *CampaignTracker) AddScenarioResult(res types.ScenarioResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scenarios = append(c.scenarios, res)
}

// CompileResult aggregates all data into a CampaignResult report.
func (c *CampaignTracker) CompileResult() types.CampaignResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	res := types.CampaignResult{
		SessionID: c.ID(),
		Passed:    true,
		Metadata:  c.metadata,
		Details:   c.scenarios,
	}

	for _, s := range c.scenarios {
		res.Summary.TotalScenarios++
		switch s.StatusVal {
		case types.StatusPass:
			res.Summary.PassedCount++
		case types.StatusFail:
			res.Summary.FailedCount++
			res.Passed = false
		case types.StatusBlocked:
			res.Summary.BlockedCount++
			res.Passed = false
		}
	}

	return res
}

func generateUUID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	// mock basic UUID format
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16])
}

// generateSecureHash creates a hash string for verification.
func generateSecureHash() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
