// Package runner orchestrates ATF scenario execution: child write/crash → reopen → verify.
package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/crash"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/errors"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/eventbus"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/evidence"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/interfaces"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/resource"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/session"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/telemetry"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/verifier"
)

// ScenarioRunner executes one acceptance scenario end-to-end.
type ScenarioRunner struct {
	logger       *logging.Logger
	eventBus     *eventbus.EventBus
	resourceMgr  *resource.ResourceManager
	telemetry    *telemetry.TelemetryStore
	subCtrl      *SubprocessController
	crashManager *crash.Manager
	evidence     *evidence.Collector
}

// SetEvidenceCollector enables failure-artifact packaging. When set, a failed
// scenario's recovered directory and verification report are zipped into the
// collector's base directory before cleanup runs.
func (sr *ScenarioRunner) SetEvidenceCollector(c *evidence.Collector) {
	sr.evidence = c
}

// NewScenarioRunner wires dependencies.
func NewScenarioRunner(
	logger *logging.Logger,
	eb *eventbus.EventBus,
	rm *resource.ResourceManager,
	ts *telemetry.TelemetryStore,
) *ScenarioRunner {
	reg, err := crash.NewBuiltinRegistry()
	if err != nil {
		// Builtins are static; failure indicates programmer error.
		panic(err)
	}
	return &ScenarioRunner{
		logger:       logger,
		eventBus:     eb,
		resourceMgr:  rm,
		telemetry:    ts,
		subCtrl:      NewSubprocessController(logger, 90*time.Second),
		crashManager: crash.NewManager(reg, logger, eb, ts),
	}
}

// Run executes write/crash subprocess, recovers, and verifies logical state.
// It never returns StatusPass unless Get+Scan verifiers succeed.
func (sr *ScenarioRunner) Run(
	ctx context.Context,
	scenario interfaces.Scenario,
	tracker *session.SessionTracker,
) (types.ScenarioResult, error) {
	id := scenario.ID()
	sr.logger.Info("ATF run start scenario=%s crash=%s", id, scenario.CrashPoint())

	fail := func(stage string, err error) (types.ScenarioResult, error) {
		_ = tracker.Transition(types.StateScenarioFailed)
		sr.telemetry.RecordMetric(id, "failures_"+stage, 1)
		return types.ScenarioResult{
			ScenarioID: id,
			StatusVal:  types.StatusFail,
		}, errors.NewExecutionError(stage, err)
	}

	if err := tracker.Transition(types.StateExecutionPrepared); err != nil {
		return fail("prepare", err)
	}

	execSession := types.ExecutionSession{
		SessionID:  tracker.ID(),
		ScenarioID: id,
		StateVal:   types.StateExecutionPrepared,
		RunIndex:   1,
		StartTime:  time.Now(),
	}

	req := types.ResourceRequest{CPUs: 1, MemoryMB: 128, FileDescriptor: 32}
	alloc, err := sr.resourceMgr.Reserve(ctx, req)
	if err != nil {
		return fail("reserve", err)
	}
	defer sr.resourceMgr.Release(alloc)

	tempDir, err := sr.resourceMgr.AllocateTempDir(id)
	if err != nil {
		return fail("tempdir", err)
	}
	execSession.TempDir = tempDir
	passed := false
	defer func() {
		_ = sr.resourceMgr.RetainOrClean(tempDir, passed)
	}()

	if err := tracker.Transition(types.StateSubprocessWriting); err != nil {
		return fail("writing", err)
	}
	execSession.StateVal = types.StateSubprocessWriting
	sr.eventBus.Publish(ctx, types.EventSubprocessStarted, execSession)

	crashDecision, err := sr.crashManager.EvaluateForScenario(
		ctx,
		execSession,
		id,
		scenario.CrashPoint(),
		map[string]string{"PEBBLEDB_FORCE_FLUSH": "1"},
	)
	if err != nil {
		return fail("crash_config", err)
	}

	execRes, err := sr.subCtrl.RunSubprocess(ctx, execSession, scenario, crashDecision)
	if err != nil {
		return fail("subprocess", err)
	}
	sr.telemetry.RecordDuration(id, "subprocess_runtime", time.Duration(execRes.Duration)*time.Millisecond)

	if execRes.ExitCode == 2 {
		if err := tracker.Transition(types.StateSubprocessCrashed); err != nil {
			return fail("crash_transition", err)
		}
		execSession.StateVal = types.StateSubprocessCrashed
		sr.eventBus.Publish(ctx, types.EventSubprocessCrashed, execSession)
	} else {
		if scenario.CrashPoint() != "" {
			return fail("crash_expected", fmt.Errorf("expected crash at %q but child exited 0", scenario.CrashPoint()))
		}
		if err := tracker.Transition(types.StateSubprocessExited); err != nil {
			return fail("exit_transition", err)
		}
		execSession.StateVal = types.StateSubprocessExited
	}

	if err := tracker.Transition(types.StateDirectorySnapshoted); err != nil {
		return fail("snapshot", err)
	}

	if err := tracker.Transition(types.StateRecoveryAttempted); err != nil {
		return fail("recovery_transition", err)
	}
	sr.eventBus.Publish(ctx, types.EventRecoveryStarted, execSession)

	if err := tracker.Transition(types.StateVerificationRunning); err != nil {
		return fail("verify_transition", err)
	}

	engine := verifier.NewVerificationEngine(
		sr.logger,
		sr.eventBus,
		sr.telemetry,
		verifier.DefaultRegistry(),
		verifier.DefaultOracleLoader(),
		verifier.DefaultEngineConfig(),
	)
	report, err := engine.Verify(verifier.Request{
		Ctx:             ctx,
		ScenarioID:      id,
		ExecutionID:     execSession.SessionID,
		DatabaseDir:     tempDir,
		VerificationDAG: scenario.VerificationDAG(),
		Config: types.Configuration{
			CompactionThreshold: -1,
		},
	})
	sr.eventBus.Publish(ctx, types.EventRecoveryFinished, execSession)
	if err != nil || report == nil || !report.Passed {
		_ = tracker.Transition(types.StateScenarioFailed)
		nFail := 0
		if report != nil {
			nFail = report.FailedChecks
		}
		result := types.ScenarioResult{
			ScenarioID:   id,
			StatusVal:    types.StatusFail,
			Executions:   []types.ExecutionResult{execRes},
			Verification: toOutcome(report),
			TempDir:      tempDir,
			FailureStage: "verify",
		}
		result.EvidencePath = sr.collectEvidence(ctx, id, execSession.SessionID, tempDir, report, execRes)
		return result, errors.NewValidationError(fmt.Sprintf("%d verification check(s) failed", nFail), err)
	}

	if err := tracker.Transition(types.StateEvidenceCollected); err != nil {
		return fail("evidence", err)
	}
	if err := tracker.Transition(types.StateScenarioCompleted); err != nil {
		return fail("complete", err)
	}

	passed = true
	execSession.EndTime = time.Now()
	sr.logger.Info("ATF run PASS scenario=%s live_keys=%d", id, report.ExpectedStatistics.ExpectedLiveKeys)
	return types.ScenarioResult{
		ScenarioID:   id,
		StatusVal:    types.StatusPass,
		Retries:      0,
		Executions:   []types.ExecutionResult{execRes},
		Verification: toOutcome(report),
		TempDir:      tempDir,
	}, nil
}

// collectEvidence packages the failed scenario's directory and report when an
// evidence collector is configured. It returns the bundle path (or "").
func (sr *ScenarioRunner) collectEvidence(
	ctx context.Context,
	scenarioID, executionID, dir string,
	report *verifier.VerificationReport,
	execRes types.ExecutionResult,
) string {
	if sr.evidence == nil {
		return ""
	}
	bundle := map[string]any{
		"scenario_id":  scenarioID,
		"execution_id": executionID,
		"execution":    execRes,
		"verification": report,
	}
	path, err := sr.evidence.Package(scenarioID, executionID, dir, bundle)
	if err != nil {
		sr.logger.Warn("ATF evidence packaging failed scenario=%s: %v", scenarioID, err)
		return ""
	}
	sr.logger.Warn("ATF evidence packaged scenario=%s bundle=%s", scenarioID, path)
	sr.eventBus.Publish(ctx, types.EventEvidenceZipped, map[string]string{
		"scenario_id":  scenarioID,
		"execution_id": executionID,
		"bundle":       path,
	})
	return path
}

// toOutcome projects a verifier report into the leaf types.VerificationOutcome
// so it can be attached to a ScenarioResult without leaking the verifier package.
func toOutcome(report *verifier.VerificationReport) *types.VerificationOutcome {
	if report == nil {
		return nil
	}
	out := &types.VerificationOutcome{
		Passed:       report.Passed,
		PassedChecks: report.PassedChecks,
		FailedChecks: report.FailedChecks,
		DurationMs:   float64(report.Duration.Microseconds()) / 1000.0,
		Aborted:      report.Aborted,
		AbortReason:  report.AbortReason,
	}
	for _, m := range report.ModuleResults {
		out.Modules = append(out.Modules, types.ModuleOutcome{
			Name:         m.Name,
			Passed:       m.Passed,
			PassedChecks: m.PassedChecks,
			FailedChecks: m.FailedChecks,
			DurationMs:   float64(m.Duration.Microseconds()) / 1000.0,
		})
	}
	for _, f := range report.Failures() {
		out.Failures = append(out.Failures, types.VerifierFailure{
			Verifier: f.Verifier,
			Key:      f.Key,
			Expected: f.ExpectedValue,
			Actual:   f.RecoveredValue,
			Details:  f.Explanation,
		})
	}
	return out
}
