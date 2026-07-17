package framework

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/config"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/eventbus"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/registry"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/resource"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/runner"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/session"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/telemetry"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

func init() {
	if os.Getenv("PEBBLEDB_CHILD_PROCESS") == "1" {
		runner.RunChildProcessMain()
	}
}

// TestATFChildNop exists solely as a spawn target for ATF subprocesses.
// Child init() exits before this body runs.
func TestATFChildNop(t *testing.T) {
	t.Helper()
}

// crashMatrix enumerates every built-in engine crash point wired end-to-end
// through the ATF: deterministic write → crash → reopen → multi-module verify.
// Flush points are reached by ForceMemtableFlush; compaction points are reached
// by producing two SSTables and then ForceCompaction inside the child.
var crashMatrix = []struct {
	id         string
	crashPoint string
	compaction bool
}{
	{"EXS-012", "flush_after_sst_close", false},
	{"EXS-010", "flush_after_manifest", false},
	{"EXS-011", "flush_after_wal_state", false},
	{"EXS-013", "flush_after_wal_truncate", false},
	{"EXS-014", "compact_after_merge_close", true},
	{"EXS-015", "compact_after_manifest", true},
	{"EXS-016", "compact_after_sstables_update", true},
	{"EXS-017", "compact_after_delete_old", true},
}

func TestATFCrashRecoveryMatrix(t *testing.T) {
	for _, tc := range crashMatrix {
		tc := tc
		t.Run(tc.crashPoint, func(t *testing.T) {
			runATFCrashScenario(t, tc.id, tc.crashPoint, tc.compaction)
		})
	}
}

func runATFCrashScenario(t *testing.T, id, crashPoint string, compaction bool) {
	t.Helper()
	base := t.TempDir()
	logFile := filepath.Join(base, "atf.log")
	f, err := os.Create(logFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	logger := logging.NewLogger(f, logging.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	loader := config.NewConfigLoader(base, []string{"--parallelism=1"})
	conf, err := loader.Load("")
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	options := map[string]string{
		"memtable_size_bytes": "1048576",
		"key_count":           "40",
		"overwrite_count":     "4",
		"tombstone_every":     "5",
		"seed":                "99",
	}
	capabilities := []string{"requires_wal", "requires_flush"}
	prefix := "FlushCrash_"
	if compaction {
		// A low threshold plus two SSTables (dataset + idempotent replay) lets the
		// child reach the compaction crash points deterministically.
		options["compaction_threshold"] = "2"
		capabilities = append(capabilities, "requires_compaction")
		prefix = "CompactCrash_"
	}

	reg := registry.NewMapRegistry()
	scenario := types.ScenarioDefinition{
		IDStr:           id,
		NameStr:         prefix + crashPoint,
		VersionStr:      "1.0.0",
		PriorityVal:     types.P1,
		RequirementsVal: []string{"DB-REC-005"},
		ContractsVal:    []string{"C-DUR-01"},
		CapabilitiesVal: capabilities,
		OptionsMap:      options,
		CrashPointStr:   crashPoint,
		VerifyDAGMap: map[string][]string{
			"metadata_verifier":   nil,
			"get_verifier":        {"metadata_verifier"},
			"iterator_verifier":   {"get_verifier"},
			"range_scan_verifier": {"get_verifier"},
			"snapshot_verifier":   {"get_verifier"},
			"directory_audit":     {"metadata_verifier"},
			"manifest_audit":      {"directory_audit"},
			"checkpoint_audit":    {"manifest_audit"},
		},
	}
	if err := reg.Register(scenario); err != nil {
		t.Fatal(err)
	}
	found, err := reg.Lookup(id)
	if err != nil {
		t.Fatal(err)
	}

	bus := eventbus.NewEventBus(logger)
	if err := bus.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer bus.Stop()

	ts := telemetry.NewTelemetryStore()
	if err := bus.Subscribe(ts); err != nil {
		t.Fatal(err)
	}

	rm := resource.NewResourceManager(logger, conf.BaseDir, 4, 1024, 100)
	rm.SetRetainArtifacts(false)

	tracker := session.NewSessionTracker(types.StateScenarioRunning)
	run := runner.NewScenarioRunner(logger, bus, rm, ts)
	result, err := run.Run(ctx, found, tracker)
	if err != nil {
		t.Fatalf("ATF run failed: %v", err)
	}
	if result.StatusVal != types.StatusPass {
		t.Fatalf("expected PASS, got %s", result.StatusVal)
	}
	if len(result.Executions) != 1 || result.Executions[0].ExitCode != 2 {
		t.Fatalf("expected child exit 2 (crash), got %+v", result.Executions)
	}
}

func TestResourceManagerCancelWhileWaiting(t *testing.T) {
	logger := logging.NewLogger(os.Stderr, logging.LevelError)
	rm := resource.NewResourceManager(logger, t.TempDir(), 1, 64, 10)
	ctx := context.Background()
	r1, err := rm.Reserve(ctx, types.ResourceRequest{CPUs: 1, MemoryMB: 1, FileDescriptor: 1})
	if err != nil {
		t.Fatal(err)
	}

	ctx2, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = rm.Reserve(ctx2, types.ResourceRequest{CPUs: 1, MemoryMB: 1, FileDescriptor: 1})
	if err == nil {
		t.Fatal("expected context cancellation while waiting")
	}
	if err := rm.Release(r1); err != nil {
		t.Fatal(err)
	}
	if err := rm.Release(r1); err != nil {
		t.Fatal(err)
	}
}
