// Package runner orchestrates the execution lifecycle steps of individual acceptance scenarios,
// advancing session states, publishing lifecycle markers, and coordinating verifiers.
//
// Dependency Rules:
// - Imports: interfaces, types, errors, logging, eventbus, telemetry, resource, session.
package runner

import (
	"context"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/errors"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/eventbus"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/interfaces"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/resource"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/session"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/telemetry"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// ScenarioRunner implements interfaces.ScenarioRunner.
type ScenarioRunner struct {
	logger      *logging.Logger
	eventBus    *eventbus.EventBus
	resourceMgr *resource.ResourceManager
	telemetry   *telemetry.TelemetryStore
}

// NewScenarioRunner allocates a ScenarioRunner.
func NewScenarioRunner(
	logger *logging.Logger,
	eb *eventbus.EventBus,
	rm *resource.ResourceManager,
	ts *telemetry.TelemetryStore,
) *ScenarioRunner {
	return &ScenarioRunner{
		logger:      logger,
		eventBus:    eb,
		resourceMgr: rm,
		telemetry:   ts,
	}
}

// Run executes the conceptual workflow of the scenario, driving state transitions and event markers.
// Actual child process launching, crash timing, and physical checks are stubbed for Milestone 1.
func (sr *ScenarioRunner) Run(
	ctx context.Context,
	scenario interfaces.Scenario,
	sess interface{},
) (interface{}, error) {
	tracker, ok := sess.(*session.SessionTracker)
	if !ok {
		return nil, errors.NewExecutionError("invalid session tracker type", nil)
	}

	id := scenario.ID()
	sr.logger.Info("Starting run execution sequence for scenario: %s", id)

	// Inject logger into context for propagation
	ctx = sr.logger.Inject(ctx)

	// 1. Prepare Execution Session State
	if err := tracker.Transition(types.StateExecutionPrepared); err != nil {
		return nil, err
	}

	execSession := types.ExecutionSession{
		SessionID:  tracker.ID(),
		ScenarioID: id,
		StateVal:   types.StateExecutionPrepared,
		RunIndex:   1,
		StartTime:  time.Now(),
	}

	// 2. Resource Request Allocation (Conceptual slots)
	req := types.ResourceRequest{
		CPUs:           1,
		MemoryMB:       64,
		FileDescriptor: 10,
	}
	alloc, err := sr.resourceMgr.Reserve(ctx, req)
	if err != nil {
		return nil, errors.NewExecutionError("resource reservation failed", err)
	}
	defer sr.resourceMgr.Release(alloc)

	// Allocate temporary sandbox path
	tempDir, err := sr.resourceMgr.AllocateTempDir(id)
	if err != nil {
		return nil, err
	}
	defer sr.resourceMgr.CleanTempDir(tempDir)
	execSession.TempDir = tempDir

	// 3. Subprocess Write Stage
	if err := tracker.Transition(types.StateSubprocessWriting); err != nil {
		return nil, err
	}
	execSession.StateVal = types.StateSubprocessWriting
	sr.eventBus.Publish(ctx, types.EventSubprocessStarted, execSession)
	sr.telemetry.RecordDuration(id, "subprocess_write", 5*time.Millisecond)

	// 4. Subprocess Crash Stage
	if err := tracker.Transition(types.StateSubprocessCrashed); err != nil {
		return nil, err
	}
	execSession.StateVal = types.StateSubprocessCrashed
	sr.eventBus.Publish(ctx, types.EventSubprocessCrashed, execSession)

	// 5. Directory Snapshot Stage
	if err := tracker.Transition(types.StateDirectorySnapshoted); err != nil {
		return nil, err
	}
	sr.telemetry.RecordDuration(id, "directory_snapshot", 2*time.Millisecond)

	// 6. Recovery Open Stage
	if err := tracker.Transition(types.StateRecoveryAttempted); err != nil {
		return nil, err
	}
	sr.telemetry.RecordDuration(id, "recovery_reopen", 10*time.Millisecond)

	// 7. Verification Sweep Stage
	if err := tracker.Transition(types.StateVerificationRunning); err != nil {
		return nil, err
	}
	sr.telemetry.RecordDuration(id, "invariant_checks", 1*time.Millisecond)

	// 8. Evidence Collection Stage
	if err := tracker.Transition(types.StateEvidenceCollected); err != nil {
		return nil, err
	}
	sr.telemetry.RecordDuration(id, "evidence_bundle", 3*time.Millisecond)

	// 9. Completion Stage
	if err := tracker.Transition(types.StateScenarioCompleted); err != nil {
		return nil, err
	}

	execSession.StateVal = types.StateScenarioCompleted
	execSession.EndTime = time.Now()

	result := types.ScenarioResult{
		ScenarioID: id,
		StatusVal:  types.StatusPass,
		Retries:    0,
		Executions: []types.ExecutionResult{
			{
				RunIndex:      1,
				ExitCode:      2,
				Duration:      float64(time.Since(execSession.StartTime).Milliseconds()),
				StderrSummary: "none (milestone 1 dry run)",
				StateAtExit:   types.StateScenarioCompleted,
			},
		},
	}

	sr.logger.Info("Scenario execution finished successfully: %s", id)
	return result, nil
}
