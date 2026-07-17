package verifier

import (
	"context"
	"os"
	"testing"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/dataset"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

type harness struct {
	dir      string
	database *db.DB
	expected *dataset.MapExpectedState
}

func openHarness(t *testing.T, seed int64, count, overwrite, tombstoneEvery int) *harness {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(db.Options{Dir: dir, CompactionThreshold: -1})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	writer := &dbWriter{db: database}
	expected, err := dataset.NewSequentialGenerator(seed, count, overwrite, tombstoneEvery).
		Generate(context.Background(), writer)
	if err != nil {
		_ = database.Close()
		t.Fatalf("generate: %v", err)
	}
	expected.ScenarioID = "test-scenario"
	expected.ExecutionID = "test-exec"
	if err := expected.Persist(dir); err != nil {
		_ = database.Close()
		t.Fatalf("persist: %v", err)
	}
	if err := database.ForceMemtableFlush(); err != nil {
		_ = database.Close()
		t.Fatalf("flush: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	database, err = db.Open(db.Options{Dir: dir, CompactionThreshold: -1})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	loaded, err := dataset.LoadExpectedState(dir)
	if err != nil {
		t.Fatalf("reload oracle: %v", err)
	}
	return &harness{dir: dir, database: database, expected: loaded}
}

type dbWriter struct {
	db *db.DB
}

func (w *dbWriter) Put(key, value []byte) error { return w.db.Put(key, value) }
func (w *dbWriter) Delete(key []byte) error     { return w.db.Delete(key) }
func (w *dbWriter) Sync() error                 { return w.db.Sync() }
func (w *dbWriter) Flush() error                { return w.db.ForceMemtableFlush() }
func (w *dbWriter) Close() error                { return nil }

func testContext(h *harness) *VerificationContext {
	report := &VerificationReport{
		ScenarioID:  h.expected.ScenarioID,
		ExecutionID: h.expected.ExecutionID,
		Passed:      true,
	}
	return &VerificationContext{
		ctx:          context.Background(),
		executionID:  h.expected.ExecutionID,
		scenarioID:   h.expected.ScenarioID,
		expected:     h.expected,
		databasePath: h.dir,
		database:     h.database,
		logger:       logging.NewLogger(os.Stderr, logging.LevelError),
		config:       types.Configuration{CompactionThreshold: -1},
		registry:     DefaultRegistry(),
		report:       report,
	}
}
