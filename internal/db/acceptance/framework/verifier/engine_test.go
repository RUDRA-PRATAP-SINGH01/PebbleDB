package verifier

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/dataset"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/eventbus"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/telemetry"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

func TestVerificationEnginePass(t *testing.T) {
	h := openHarness(t, 61, 40, 5, 5)
	_ = h.database.Close()

	logger := logging.NewLogger(os.Stderr, logging.LevelError)
	bus := eventbus.NewEventBus(logger)
	_ = bus.Start(context.Background())
	defer bus.Stop()
	ts := telemetry.NewTelemetryStore()

	engine := NewVerificationEngine(logger, bus, ts, DefaultRegistry(), DefaultOracleLoader(), DefaultEngineConfig())
	report, err := engine.Verify(Request{
		Ctx:         context.Background(),
		ScenarioID:  "test-scenario",
		ExecutionID: "test-exec",
		DatabaseDir: h.dir,
		Config:      types.Configuration{CompactionThreshold: -1},
	})
	if err != nil {
		t.Fatalf("verify: %v report=%+v", err, report)
	}
	if !report.Passed {
		t.Fatalf("expected pass: %+v", report.FailureSummary)
	}
	if len(report.ModuleResults) < 5 {
		t.Fatalf("expected >=5 modules, got %d", len(report.ModuleResults))
	}
	if report.Duration <= 0 {
		t.Fatal("expected duration")
	}
}

func TestVerificationEngineCorruptOracle(t *testing.T) {
	h := openHarness(t, 62, 10, 0, 0)
	_ = h.database.Close()

	path := filepath.Join(h.dir, dataset.ExpectedStateFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["checksum"] = json.RawMessage(`"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"`)
	out, _ := json.MarshalIndent(raw, "", "  ")
	if err := os.WriteFile(path, out, 0644); err != nil {
		t.Fatal(err)
	}

	engine := NewVerificationEngine(
		logging.NewLogger(os.Stderr, logging.LevelError),
		nil, nil, DefaultRegistry(), DefaultOracleLoader(), DefaultEngineConfig(),
	)
	report, err := engine.Verify(Request{
		Ctx:         context.Background(),
		ScenarioID:  "test-scenario",
		ExecutionID: "test-exec",
		DatabaseDir: h.dir,
	})
	if err == nil || report == nil || !report.Aborted {
		t.Fatalf("expected abort on corrupt oracle, err=%v report=%+v", err, report)
	}
}

func TestVerificationEngineDetectsLogicalFailure(t *testing.T) {
	h := openHarness(t, 63, 20, 0, 0)
	live := h.expected.LiveKeys()
	if err := h.database.Delete(live[0]); err != nil {
		t.Fatal(err)
	}
	_ = h.database.Sync()
	_ = h.database.Close()

	engine := NewVerificationEngine(
		logging.NewLogger(os.Stderr, logging.LevelError),
		nil, nil, DefaultRegistry(), DefaultOracleLoader(), EngineConfig{IdempotentReopens: 1, DisableDefaultCompaction: true},
	)
	report, err := engine.Verify(Request{
		Ctx:         context.Background(),
		ScenarioID:  "test-scenario",
		ExecutionID: "test-exec",
		DatabaseDir: h.dir,
	})
	if err == nil || report.Passed {
		t.Fatalf("expected logical failure, err=%v passed=%v", err, report.Passed)
	}
}

func TestVerificationEngineIdempotentReopen(t *testing.T) {
	h := openHarness(t, 64, 15, 2, 5)
	_ = h.database.Close()

	engine := NewVerificationEngine(
		logging.NewLogger(os.Stderr, logging.LevelError),
		nil, nil, DefaultRegistry(), DefaultOracleLoader(),
		EngineConfig{IdempotentReopens: 3, DisableDefaultCompaction: true},
	)
	report, err := engine.Verify(Request{
		Ctx:         context.Background(),
		ScenarioID:  "test-scenario",
		ExecutionID: "test-exec",
		DatabaseDir: h.dir,
	})
	if err != nil || !report.Passed {
		t.Fatalf("err=%v summary=%v", err, report.FailureSummary)
	}
	reopens := 0
	for _, m := range report.ModuleResults {
		if len(m.Name) >= len("idempotent_reopen_") && m.Name[:len("idempotent_reopen_")] == "idempotent_reopen_" {
			reopens++
		}
	}
	if reopens != 3 {
		t.Fatalf("expected 3 idempotent modules, got %d", reopens)
	}
}

func TestDefaultRegistryOrder(t *testing.T) {
	names := DefaultRegistry().Names()
	want := []string{
		"metadata_verifier",
		"get_verifier",
		"iterator_verifier",
		"range_scan_verifier",
		"snapshot_verifier",
	}
	if len(names) != len(want) {
		t.Fatalf("names=%v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("order[%d]=%s want %s", i, names[i], want[i])
		}
	}
}

func TestVerificationEngineEmptyOracleFreshDB(t *testing.T) {
	dir := t.TempDir()
	state := dataset.NewMapExpectedState(1, 0)
	state.ScenarioID = "s"
	state.ExecutionID = "e"
	state.State = map[string]types.ValueSnapshot{}
	if err := state.Persist(dir); err != nil {
		t.Fatal(err)
	}
	engine := NewVerificationEngine(
		logging.NewLogger(os.Stderr, logging.LevelError),
		nil, nil, DefaultRegistry(), DefaultOracleLoader(),
		EngineConfig{IdempotentReopens: 1, DisableDefaultCompaction: true},
	)
	report, err := engine.Verify(Request{
		Ctx:         context.Background(),
		ScenarioID:  "s",
		ExecutionID: "e",
		DatabaseDir: dir,
	})
	if err != nil || report == nil || !report.Passed {
		t.Fatalf("empty oracle + fresh DB should pass: err=%v report=%+v", err, report)
	}
	_ = db.ErrNotFound
}
