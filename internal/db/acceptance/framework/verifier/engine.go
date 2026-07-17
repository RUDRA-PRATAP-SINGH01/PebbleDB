package verifier

import (
	"context"
	"fmt"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/dataset"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/errors"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/eventbus"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/telemetry"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// EngineConfig configures VerificationEngine behavior.
type EngineConfig struct {
	// IdempotentReopens is the number of close/reopen+Get cycles after modules (default 3).
	IdempotentReopens int
	// DisableDefaultCompaction mirrors recovery harness (CompactionThreshold=-1).
	DisableDefaultCompaction bool
}

// DefaultEngineConfig returns production defaults.
func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		IdempotentReopens:        3,
		DisableDefaultCompaction: true,
	}
}

// Request is the input to VerificationEngine.Verify.
type Request struct {
	Ctx         context.Context
	ScenarioID  string
	ExecutionID string
	DatabaseDir string
	Config      types.Configuration
	// VerificationDAG is the scenario's module dependency graph (module → deps).
	// When non-empty it controls module execution order and lets the engine skip
	// modules whose dependencies failed. Unknown module names are ignored so the
	// DAG stays advisory and backward compatible.
	VerificationDAG map[string][]string
}

// VerificationEngine orchestrates oracle load, DB open, verifier modules, and reporting.
type VerificationEngine struct {
	logger    *logging.Logger
	eventBus  *eventbus.EventBus
	telemetry *telemetry.TelemetryStore
	registry  *Registry
	loader    *OracleLoader
	engineCfg EngineConfig
}

// NewVerificationEngine constructs an engine with dependency injection.
func NewVerificationEngine(
	logger *logging.Logger,
	eb *eventbus.EventBus,
	ts *telemetry.TelemetryStore,
	registry *Registry,
	loader *OracleLoader,
	cfg EngineConfig,
) *VerificationEngine {
	if registry == nil {
		registry = DefaultRegistry()
	}
	if loader == nil {
		loader = DefaultOracleLoader()
	}
	if cfg.IdempotentReopens < 0 {
		cfg.IdempotentReopens = 0
	}
	return &VerificationEngine{
		logger:    logger,
		eventBus:  eb,
		telemetry: ts,
		registry:  registry,
		loader:    loader,
		engineCfg: cfg,
	}
}

// Verify loads the oracle, opens the recovered DB, runs all registered verifiers,
// performs idempotent reopen checks, and returns a deterministic report.
func (e *VerificationEngine) Verify(req Request) (*VerificationReport, error) {
	start := time.Now()
	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	report := &VerificationReport{
		ScenarioID:  req.ScenarioID,
		ExecutionID: req.ExecutionID,
		Passed:      true,
	}

	e.publish(ctx, types.EventVerificationStarted, map[string]string{
		"scenario_id":  req.ScenarioID,
		"execution_id": req.ExecutionID,
		"dir":          req.DatabaseDir,
	})

	loader := e.loader
	if req.ScenarioID != "" || req.ExecutionID != "" {
		opts := loader.opts
		if req.ScenarioID != "" {
			opts.ExpectedScenarioID = req.ScenarioID
		}
		if req.ExecutionID != "" {
			opts.ExpectedExecutionID = req.ExecutionID
		}
		loader = NewOracleLoader(opts)
	}

	expected, err := loader.Load(req.DatabaseDir)
	if err != nil {
		report.Passed = false
		report.Aborted = true
		report.AbortReason = err.Error()
		report.Duration = time.Since(start)
		e.publish(ctx, types.EventVerificationAborted, report)
		e.metric(req.ScenarioID, "verification_failures", 1)
		return report, err
	}

	openStart := time.Now()
	opts := db.Options{Dir: req.DatabaseDir}
	if e.engineCfg.DisableDefaultCompaction {
		opts.CompactionThreshold = -1
	} else if req.Config.CompactionThreshold != 0 {
		opts.CompactionThreshold = req.Config.CompactionThreshold
	}
	if req.Config.MemtableSizeBytes > 0 {
		opts.MemtableSize = req.Config.MemtableSizeBytes
	}
	opts.SyncWrites = req.Config.SyncWrites

	database, err := db.Open(opts)
	report.DatabaseOpenTime = time.Since(openStart)
	e.duration(req.ScenarioID, "database_open", report.DatabaseOpenTime)
	if err != nil {
		report.Passed = false
		report.Aborted = true
		report.AbortReason = err.Error()
		report.DatabaseMetadata = DatabaseMetadata{
			OpenedCleanly: false,
			Dir:           req.DatabaseDir,
		}
		report.Duration = time.Since(start)
		e.publish(ctx, types.EventVerificationAborted, report)
		e.metric(req.ScenarioID, "verification_failures", 1)
		return report, errors.NewExecutionError("database open failed", err)
	}

	report.DatabaseMetadata.OpenedCleanly = true
	report.DatabaseMetadata.Dir = req.DatabaseDir
	report.DatabaseMetadata.CompactionDisabled = e.engineCfg.DisableDefaultCompaction
	report.ExpectedStatistics = Statistics{
		ExpectedLiveKeys:   expected.LiveCount(),
		ExpectedTombstones: countTombstones(expected),
		OracleSeed:         expected.Seed,
		OracleCount:        expected.Count,
	}

	vctx := &VerificationContext{
		ctx:          ctx,
		executionID:  req.ExecutionID,
		scenarioID:   req.ScenarioID,
		expected:     expected,
		databasePath: req.DatabaseDir,
		database:     database,
		logger:       e.logger,
		telemetry:    e.telemetry,
		config:       req.Config,
		eventBus:     e.eventBus,
		registry:     e.registry,
		report:       report,
	}

	order, err := resolveModuleOrder(e.registry, req.VerificationDAG)
	if err != nil {
		_ = database.Close()
		report.Passed = false
		report.Aborted = true
		report.AbortReason = err.Error()
		report.Duration = time.Since(start)
		e.publish(ctx, types.EventVerificationAborted, report)
		e.metric(req.ScenarioID, "verification_failures", 1)
		return report, errors.NewValidationError("verifier dag resolution failed", err)
	}

	var totalKeys int64
	var totalScans int64
	var totalFailures float64
	unhealthy := make(map[string]bool)

	for _, mod := range order.modules {
		if err := ctx.Err(); err != nil {
			_ = database.Close()
			report.Passed = false
			report.Aborted = true
			report.AbortReason = err.Error()
			report.Duration = time.Since(start)
			e.publish(ctx, types.EventVerificationAborted, report)
			return report, err
		}

		// Skip a module whose dependency already failed/skipped: its result would
		// be meaningless and the report is already marked failed by the dependency.
		if blockedBy := firstUnhealthyDep(order.deps[mod.Name()], unhealthy); blockedBy != "" {
			unhealthy[mod.Name()] = true
			skipped := emptyModuleResult(mod.Name())
			skipped.Warnings = 1
			skipped.Failures = append(skipped.Failures, Failure{
				Verifier:       mod.Name(),
				ExpectedValue:  "dependency satisfied",
				RecoveredValue: fmt.Sprintf("dependency %q unhealthy", blockedBy),
				Reason:         "dependency_skipped",
				Severity:       SeverityWarning,
				RecoveryPhase:  PhaseVerify,
				Explanation:    fmt.Sprintf("Module %q skipped because dependency %q failed", mod.Name(), blockedBy),
			})
			report.addModule(*skipped)
			continue
		}

		e.publish(ctx, types.EventVerifierStarted, map[string]string{
			"verifier":    mod.Name(),
			"scenario_id": req.ScenarioID,
		})
		modStart := time.Now()
		result, verr := mod.Verify(vctx)
		if result == nil {
			result = emptyModuleResult(mod.Name())
			result.Passed = false
			result.Failures = append(result.Failures, newFailure(
				mod.Name(), "", "module result", "nil", "nil_result",
				"Verifier returned a nil ModuleResult",
			))
		}
		if result.Duration == 0 {
			result.Duration = time.Since(modStart)
		}
		e.duration(req.ScenarioID, "verifier_"+mod.Name(), result.Duration)
		totalKeys += result.KeysVerified
		totalScans += result.ScansPerformed

		if verr != nil {
			report.addModule(*result)
			_ = database.Close()
			report.Passed = false
			report.Aborted = true
			report.AbortReason = verr.Error()
			report.Duration = time.Since(start)
			e.publish(ctx, types.EventVerifierFailed, result)
			e.publish(ctx, types.EventVerificationAborted, report)
			e.metric(req.ScenarioID, "verification_failures", 1)
			return report, verr
		}

		report.addModule(*result)
		if result.Passed {
			e.publish(ctx, types.EventVerifierPassed, result)
		} else {
			unhealthy[mod.Name()] = true
			totalFailures += float64(len(result.Failures))
			e.publish(ctx, types.EventVerifierFailed, result)
		}
	}

	recoveredLive, err := countRecoveredLive(database)
	if err != nil {
		_ = database.Close()
		report.Passed = false
		report.Duration = time.Since(start)
		return report, err
	}
	report.RecoveredStatistics = Statistics{
		ExpectedLiveKeys:   report.ExpectedStatistics.ExpectedLiveKeys,
		ExpectedTombstones: report.ExpectedStatistics.ExpectedTombstones,
		RecoveredLiveKeys:  recoveredLive,
		OracleSeed:         expected.Seed,
		OracleCount:        expected.Count,
	}

	if err := database.Close(); err != nil {
		report.Passed = false
		report.Aborted = true
		report.AbortReason = err.Error()
		report.Duration = time.Since(start)
		e.publish(ctx, types.EventVerificationAborted, report)
		return report, errors.NewExecutionError("close recovered database", err)
	}

	if report.Passed && e.engineCfg.IdempotentReopens > 0 {
		if err := e.idempotentReopen(ctx, req, expected, report); err != nil {
			report.Passed = false
			report.Duration = time.Since(start)
			e.publish(ctx, types.EventVerificationFinished, report)
			e.metric(req.ScenarioID, "verification_failures", 1)
			return report, err
		}
	}

	report.Duration = time.Since(start)
	e.duration(req.ScenarioID, "verification_total", report.Duration)
	e.metric(req.ScenarioID, "keys_verified", float64(totalKeys))
	e.metric(req.ScenarioID, "scans_performed", float64(totalScans))
	e.metric(req.ScenarioID, "verification_failures", totalFailures)
	if !report.Passed {
		e.metric(req.ScenarioID, "verification_failed", 1)
	} else {
		e.metric(req.ScenarioID, "verification_passed", 1)
	}

	e.publish(ctx, types.EventVerificationFinished, report)
	if !report.Passed {
		return report, errors.NewValidationError(
			fmt.Sprintf("%d verification check(s) failed", report.FailedChecks),
			nil,
		)
	}
	return report, nil
}

func (e *VerificationEngine) idempotentReopen(
	ctx context.Context,
	req Request,
	expected *dataset.MapExpectedState,
	report *VerificationReport,
) error {
	getMod := GetVerifier{}
	for i := 0; i < e.engineCfg.IdempotentReopens; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		openStart := time.Now()
		again, err := db.Open(db.Options{
			Dir:                 req.DatabaseDir,
			CompactionThreshold: -1,
		})
		e.duration(req.ScenarioID, fmt.Sprintf("idempotent_open_%d", i), time.Since(openStart))
		if err != nil {
			fail := Failure{
				Verifier:       getVerifierName,
				ExpectedValue:  "successful reopen",
				RecoveredValue: err.Error(),
				Reason:         "reopen_failed",
				Severity:       SeverityError,
				RecoveryPhase:  PhaseIdempotentReopen,
				Explanation:    fmt.Sprintf("Idempotent reopen #%d failed", i),
			}
			report.addModule(ModuleResult{
				Name:         fmt.Sprintf("idempotent_reopen_%d", i),
				Passed:       false,
				Failures:     []Failure{fail},
				FailedChecks: 1,
			})
			return errors.NewExecutionError(fail.Explanation, err)
		}

		vctx := &VerificationContext{
			ctx:          ctx,
			executionID:  req.ExecutionID,
			scenarioID:   req.ScenarioID,
			expected:     expected,
			databasePath: req.DatabaseDir,
			database:     again,
			logger:       e.logger,
			telemetry:    e.telemetry,
			config:       req.Config,
			eventBus:     e.eventBus,
			registry:     e.registry,
			report:       report,
		}
		result, verr := getMod.Verify(vctx)
		_ = again.Close()
		if result == nil {
			result = emptyModuleResult(fmt.Sprintf("idempotent_reopen_%d", i))
		}
		result.Name = fmt.Sprintf("idempotent_reopen_%d", i)
		if verr != nil || !result.Passed {
			report.addModule(*result)
			return errors.NewValidationError(
				fmt.Sprintf("idempotent reopen #%d verification failed", i),
				verr,
			)
		}
		report.addModule(*result)
	}
	return nil
}

func countRecoveredLive(database *db.DB) (int, error) {
	it, err := database.Scan(nil, nil)
	if err != nil {
		return 0, err
	}
	defer it.Close()
	n := 0
	for it.Valid() {
		n++
		if err := it.Next(); err != nil {
			return n, err
		}
	}
	return n, nil
}

func (e *VerificationEngine) publish(ctx context.Context, et types.EventType, payload any) {
	if e.eventBus == nil {
		return
	}
	e.eventBus.Publish(ctx, et, payload)
}

func (e *VerificationEngine) duration(scenarioID, stage string, d time.Duration) {
	if e.telemetry == nil {
		return
	}
	e.telemetry.RecordDuration(scenarioID, stage, d)
}

func (e *VerificationEngine) metric(scenarioID, name string, value float64) {
	if e.telemetry == nil {
		return
	}
	e.telemetry.RecordMetric(scenarioID, name, value)
}
