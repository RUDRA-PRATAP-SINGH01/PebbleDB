package session

import (
	"testing"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

func TestCrashPathTransitions(t *testing.T) {
	s := NewSessionTracker(types.StateScenarioRunning)
	steps := []types.State{
		types.StateExecutionPrepared,
		types.StateSubprocessWriting,
		types.StateSubprocessCrashed,
		types.StateDirectorySnapshoted,
		types.StateRecoveryAttempted,
		types.StateVerificationRunning,
		types.StateEvidenceCollected,
		types.StateScenarioCompleted,
	}
	for _, st := range steps {
		if err := s.Transition(st); err != nil {
			t.Fatalf("%s: %v", st, err)
		}
	}
}

func TestCleanExitPathTransitions(t *testing.T) {
	s := NewSessionTracker(types.StateScenarioRunning)
	steps := []types.State{
		types.StateExecutionPrepared,
		types.StateSubprocessWriting,
		types.StateSubprocessExited,
		types.StateDirectorySnapshoted,
		types.StateRecoveryAttempted,
		types.StateVerificationRunning,
		types.StateEvidenceCollected,
		types.StateScenarioCompleted,
	}
	for _, st := range steps {
		if err := s.Transition(st); err != nil {
			t.Fatalf("%s: %v", st, err)
		}
	}
}

func TestForbiddenWritingToSnapshot(t *testing.T) {
	s := NewSessionTracker(types.StateSubprocessWriting)
	if err := s.Transition(types.StateDirectorySnapshoted); err == nil {
		t.Fatal("Writing -> Snapshot must be forbidden")
	}
}
