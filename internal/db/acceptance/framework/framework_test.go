package framework

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/config"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/eventbus"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/registry"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/resource"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/runner"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/scheduler"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/session"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/telemetry"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// MockSubscriber receives lifecycle events to verify bus dispatching.
type MockSubscriber struct {
	mu     sync.Mutex
	events []types.Event
}

// Name returns the identifier of the subscriber.
func (m *MockSubscriber) Name() string {
	return "mock_subscriber"
}

// OnEvent saves events in thread-safe slice.
func (m *MockSubscriber) OnEvent(ctx context.Context, event interface{}) error {
	ev, ok := event.(types.Event)
	if !ok {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, ev)
	return nil
}

func TestFrameworkFoundationBuildAndExecution(t *testing.T) {
	// Initialize core leaf systems
	logFile := filepath.Join(t.TempDir(), "atf.log")
	f, err := os.Create(logFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	logger := logging.NewLogger(f, logging.LevelDebug)
	ctx := context.Background()

	// 1. Config validation
	loader := config.NewConfigLoader(t.TempDir(), []string{"--parallelism=2"})
	conf, err := loader.Load("")
	if err != nil {
		t.Fatalf("config load failed: %v", err)
	}
	if conf.Parallelism != 2 {
		t.Fatalf("config CLI override failed: got parallelism %d, want 2", conf.Parallelism)
	}

	// 2. Registry checks
	reg := registry.NewMapRegistry()
	mockScenario := types.ScenarioDefinition{
		IDStr:           "EXS-001",
		NameStr:         "WALOnlyRecovery_MemtableFrozen",
		VersionStr:      "1.0.0",
		PriorityVal:     types.P1,
		RequirementsVal: []string{"DB-REC-001"},
		ContractsVal:    []string{"C-DUR-01"},
		CapabilitiesVal: []string{"requires_wal"},
		OptionsMap:      make(map[string]interface{}),
		CrashPointStr:   "flush_after_manifest",
		VerifyDAGMap:    make(map[string][]string),
	}

	if err := reg.Register(mockScenario); err != nil {
		t.Fatalf("registry register failed: %v", err)
	}

	found, err := reg.Lookup("EXS-001")
	if err != nil {
		t.Fatalf("registry lookup failed: %v", err)
	}
	if found.Name() != "WALOnlyRecovery_MemtableFrozen" {
		t.Fatalf("wrong scenario lookup result name: %s", found.Name())
	}

	// 3. EventBus start and subscriber registration
	bus := eventbus.NewEventBus(logger)
	if err := bus.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer bus.Stop()

	sub := &MockSubscriber{}
	if err := bus.Subscribe(sub); err != nil {
		t.Fatal(err)
	}

	// 4. Telemetry and Resource setup
	ts := telemetry.NewTelemetryStore()
	if err := bus.Subscribe(ts); err != nil {
		t.Fatal(err)
	}

	rm := resource.NewResourceManager(logger, conf.BaseDir, 4, 1024, 100)

	// 5. Scheduler queue submit and sort
	sched := scheduler.NewCampaignScheduler(logger, bus, rm, conf.Parallelism)
	if err := sched.Submit(mockScenario); err != nil {
		t.Fatal(err)
	}

	if err := sched.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer sched.Stop()

	// 6. Session execution run through runner
	campaign := session.NewCampaignTracker(types.Metadata{
		PebbleCommit: "beefdead",
		GoVersion:    "1.25",
		Platform:     "windows",
		Timestamp:    time.Now(),
	})
	if err := campaign.Transition(types.StateCampaignRunning); err != nil {
		t.Fatal(err)
	}

	scenTracker := session.NewSessionTracker(types.StateScenarioRunning)
	campaign.AddScenarioResult(types.ScenarioResult{
		ScenarioID: found.ID(),
		StatusVal:  types.StatusPass,
	})

	run := runner.NewScenarioRunner(logger, bus, rm, ts)
	execResultVal, err := run.Run(ctx, found, scenTracker)
	if err != nil {
		t.Fatalf("runner execution failed: %v", err)
	}

	execRes, ok := execResultVal.(types.ScenarioResult)
	if !ok {
		t.Fatalf("expected ScenarioResult execution outcome, got %T", execResultVal)
	}
	if execRes.StatusVal != types.StatusPass {
		t.Fatalf("scenario failed: expected StatusPass, got %s", execRes.StatusVal)
	}

	// Wait briefly for asynchronous subscriber processing
	time.Sleep(50 * time.Millisecond)

	// Verify metrics capture
	metricsVal := ts.Dump()
	metricsReport, ok := metricsVal.(telemetry.CampaignSummaryReport)
	if !ok {
		t.Fatalf("expected CampaignSummaryReport, got %T", metricsVal)
	}
	scStats, exists := metricsReport.Scenarios["EXS-001"]
	if !exists {
		t.Fatal("metrics report missing EXS-001 stats")
	}
	if scStats.Counters["subprocess_restarts"] != 1.0 {
		t.Fatalf("telemetry missing subprocess_restarts count: got %v, want 1.0", scStats.Counters["subprocess_restarts"])
	}

	// Verify Campaign compiled report passes
	if err := campaign.Transition(types.StateCampaignCompleted); err != nil {
		t.Fatal(err)
	}
	campResult := campaign.CompileResult()
	if !campResult.Passed {
		t.Fatal("compiled campaign result reports failure")
	}
}
