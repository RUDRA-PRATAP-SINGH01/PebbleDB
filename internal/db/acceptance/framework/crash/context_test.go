package crash

import (
	"testing"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

func TestCrashContextImmutableCopies(t *testing.T) {
	env := map[string]string{"A": "1"}
	point := CrashPoint{
		ID: "p", Name: "P", Category: CategoryFlush, Phase: PhaseFlushSST,
		EngineHook: "h", Enabled: true, Metadata: map[string]string{"k": "v"},
	}
	ctx := NewCrashContext(CrashContextParams{
		ExecutionID: "e1", ScenarioID: "s1", DatabasePath: "/db",
		ExecutionState: types.StateSubprocessWriting,
		CrashPoint:     point, Environment: env, ScenarioCrashID: "h",
		WorkingDir: "/wd", Child: ChildProcessInfo{Binary: "bin", PID: 7},
	})
	env["A"] = "mutated"
	point.Metadata["k"] = "mutated"
	if ctx.Environment()["A"] != "1" {
		t.Fatal("environment leaked mutation")
	}
	if ctx.CrashPoint().Metadata["k"] != "v" {
		t.Fatal("metadata leaked mutation")
	}
	if ctx.ExecutionID() != "e1" || ctx.ScenarioID() != "s1" {
		t.Fatal("ids")
	}
	if ctx.Child().PID != 7 || ctx.WorkingDir() != "/wd" {
		t.Fatal("child/workdir")
	}
}
