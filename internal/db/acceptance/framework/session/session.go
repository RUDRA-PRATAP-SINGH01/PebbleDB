// Package session enforces scenario/campaign lifecycle state machines.
package session

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/errors"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// AllowedTransitions maps each state to legal successors.
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
		types.StateSubprocessExited,
		types.StateScenarioFailed,
	},
	types.StateSubprocessCrashed: {
		types.StateDirectorySnapshoted,
		types.StateScenarioFailed,
	},
	types.StateSubprocessExited: {
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

// SessionTracker is a thread-safe lifecycle tracker.
type SessionTracker struct {
	mu        sync.RWMutex
	sessionID string
	state     types.State
	createdAt time.Time
}

// NewSessionTracker allocates a tracker with a random ID.
func NewSessionTracker(initialState types.State) *SessionTracker {
	return &SessionTracker{
		sessionID: generateUUID(),
		state:     initialState,
		createdAt: time.Now(),
	}
}

// ID returns the session identifier.
func (s *SessionTracker) ID() string { return s.sessionID }

// State returns the current lifecycle state.
func (s *SessionTracker) State() types.State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// CreatedAt returns creation time.
func (s *SessionTracker) CreatedAt() time.Time { return s.createdAt }

// Transition moves to newState if allowed.
func (s *SessionTracker) Transition(newState types.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	allowed, ok := AllowedTransitions[s.state]
	if !ok {
		return errors.NewStateError(fmt.Sprintf("stuck state: %s", s.state), nil)
	}
	for _, a := range allowed {
		if a == newState {
			s.state = newState
			return nil
		}
	}
	return errors.NewStateError(fmt.Sprintf("forbidden transition: %s -> %s", s.state, newState), nil)
}

// MustTransition is Transition that panics only in tests via helper — prefer Transition.
func (s *SessionTracker) Fail(reason error) error {
	_ = reason
	return s.Transition(types.StateScenarioFailed)
}

// CampaignTracker aggregates scenario results for a campaign.
type CampaignTracker struct {
	*SessionTracker
	metadata  types.Metadata
	scenarios []types.ScenarioResult
	mu        sync.Mutex
}

// NewCampaignTracker creates a campaign-level tracker.
func NewCampaignTracker(meta types.Metadata) *CampaignTracker {
	return &CampaignTracker{
		SessionTracker: NewSessionTracker(types.StateInit),
		metadata:       meta,
		scenarios:      make([]types.ScenarioResult, 0),
	}
}

// AddScenarioResult appends a scenario outcome.
func (c *CampaignTracker) AddScenarioResult(res types.ScenarioResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scenarios = append(c.scenarios, res)
}

// CompileResult builds the campaign report.
func (c *CampaignTracker) CompileResult() types.CampaignResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	res := types.CampaignResult{
		SessionID: c.ID(),
		Passed:    true,
		Metadata:  c.metadata,
		Details:   append([]types.ScenarioResult(nil), c.scenarios...),
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
		case types.StatusInconclusive:
			res.Passed = false
		}
	}
	return res
}

func generateUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
