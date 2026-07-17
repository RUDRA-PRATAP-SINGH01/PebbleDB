// Package runner orchestrates the execution lifecycle of individual acceptance scenarios,
// advancing session states, publishing lifecycle markers, and coordinating subprocess controllers.
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

// ScenarioRunner orchestrates the subprocess execution campaign for a scenario.
type ScenarioRunner struct {
	logger      *logging.Logger
	eventBus    *eventbus.EventBus
	resourceMgr *resource.ResourceManager
	telemetry   *telemetry.TelemetryStore
	subCtrl     *SubprocessController
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
		subCtrl:     NewSubprocessController(logger, 30*time.Second),
	}
}

// Run executes the complete child process run, transitioning states and publishing events.
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
	sr.logger.Info("Executing run campaign for scenario: %s", id)

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

	// 2. Resource Allocation
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

	// Allocate temporary sandbox folder namespace
	tempDir, err := sr.resourceMgr.AllocateTempDir(id)
	if err != nil {
		return nil, err
	}
	defer sr.resourceMgr.CleanTempDir(tempDir)
	execSession.TempDir = tempDir

	// 3. Subprocess Write/Execution Stage
	if err := tracker.Transition(types.StateSubprocessWriting); err != nil {
		return nil, err
	}
	execSession.StateVal = types.StateSubprocessWriting

	// Publish Event indicating subprocess starting
	sr.eventBus.Publish(ctx, types.EventSubprocessStarted, execSession)

	// Launch Child Subprocess
	execRes, err := sr.subCtrl.RunSubprocess(ctx, execSession, scenario)
	if err != nil {
		_ = tracker.Transition(types.StateScenarioFailed)
		sr.telemetry.RecordMetric(id, "subprocess_failures", 1.0)
		return nil, err
	}

	sr.telemetry.RecordDuration(id, "subprocess_runtime", time.Duration(execRes.Duration)*time.Millisecond)

	// 4. Subprocess Crashed or Completed exit check
	if execRes.ExitCode == 2 {
		if err := tracker.Transition(types.StateSubprocessCrashed); err != nil {
			return nil, err
		}
		execSession.StateVal = types.StateSubprocessCrashed
		sr.eventBus.Publish(ctx, types.EventSubprocessCrashed, execSession)
	}

	// 5. Directory Snapshot (conceptual log / snapshot placeholder for Milestone 2)
	if err := tracker.Transition(types.StateDirectorySnapshoted); err != nil {
		return nil, err
	}
	sr.telemetry.RecordDuration(id, "directory_snapshot", 1*time.Millisecond)

	// 6. Recovery Reopen Stage (mock for Milestone 2)
	if err := tracker.Transition(types.StateRecoveryAttempted); err != nil {
		return nil, err
	}

	// 7. Verification Sweep Stage (mock for Milestone 2)
	if err := tracker.Transition(types.StateVerificationRunning); err != nil {
		return nil, err
	}

	// 8. Evidence Collection Stage (mock for Milestone 2)
	if err := tracker.Transition(types.StateEvidenceCollected); err != nil {
		return nil, err
	}

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
		Executions: []types.ExecutionResult{execRes},
	}

	sr.logger.Info("Scenario execution finished successfully: %s", id)
	return result, nil
}
