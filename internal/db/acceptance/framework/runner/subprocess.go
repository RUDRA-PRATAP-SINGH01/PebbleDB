package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/crash"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/errors"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/interfaces"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

const maxCaptureBytes = 1 << 20 // 1 MiB stderr/stdout cap

// SubprocessController spawns isolated ATF child processes.
type SubprocessController struct {
	logger  *logging.Logger
	timeout time.Duration
}

// NewSubprocessController constructs a controller.
func NewSubprocessController(logger *logging.Logger, timeout time.Duration) *SubprocessController {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &SubprocessController{logger: logger, timeout: timeout}
}

// RunSubprocess executes the child protocol and returns exit metadata.
// crashDecision comes from crash.Manager; EngineHook maps to PEBBLEDB_CRASH_AT.
func (s *SubprocessController) RunSubprocess(
	ctx context.Context,
	execSess types.ExecutionSession,
	scenario interfaces.Scenario,
	crashDecision crash.Decision,
) (types.ExecutionResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	crashAt := crashDecision.EngineHook
	if !crashDecision.ShouldCrash {
		crashAt = ""
	}

	cmd := exec.CommandContext(runCtx, os.Args[0], "-test.run=^TestATFChildNop$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		"PEBBLEDB_CHILD_PROCESS=1",
		"PEBBLEDB_SCENARIO_ID="+scenario.ID(),
		"PEBBLEDB_EXECUTION_ID="+execSess.SessionID,
		"PEBBLEDB_TEST_DIR="+execSess.TempDir,
		crash.EnvKeyCrashAt+"="+crashAt,
		"PEBBLEDB_FORCE_FLUSH=1",
	)

	opts := scenario.Options()
	for k, envKey := range map[string]string{
		"memtable_size_bytes":  "PEBBLEDB_MEMTABLE_SIZE",
		"compaction_threshold": "PEBBLEDB_COMPACTION_THRESHOLD",
		"sync_writes":          "PEBBLEDB_SYNC_WRITES",
		"seed":                 "PEBBLEDB_SEED",
		"key_count":            "PEBBLEDB_KEY_COUNT",
		"overwrite_count":      "PEBBLEDB_OVERWRITE_COUNT",
		"tombstone_every":      "PEBBLEDB_TOMBSTONE_EVERY",
	} {
		if val, ok := opts[k]; ok && val != "" {
			cmd.Env = append(cmd.Env, envKey+"="+val)
		}
	}

	var stdoutBuf, stderrBuf limitedBuffer
	stdoutBuf.limit = maxCaptureBytes
	stderrBuf.limit = maxCaptureBytes
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	s.logger.Debug("Spawning ATF child scenario=%s dir=%s crash=%s decision=%v", scenario.ID(), execSess.TempDir, crashAt, crashDecision.ShouldCrash)

	start := time.Now()
	err := cmd.Run()
	duration := float64(time.Since(start).Milliseconds())
	stderrStr := stderrBuf.String()

	exitCode := 0
	if err != nil {
		if runCtx.Err() != nil {
			return types.ExecutionResult{
				RunIndex:      execSess.RunIndex,
				ExitCode:      -1,
				Duration:      duration,
				StderrSummary: truncate(stderrStr, 512),
				StateAtExit:   types.StateScenarioFailed,
			}, errors.NewExecutionError("subprocess timed out or canceled", runCtx.Err())
		}
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			return types.ExecutionResult{}, errors.NewExecutionError("subprocess failed", err)
		}
	}

	s.logger.Debug("Child exited scenario=%s exit=%d dur=%.0fms", scenario.ID(), exitCode, duration)

	// Exit 2 = intentional crash point; exit 0 = clean survival (only when no crash requested).
	if exitCode != 0 && exitCode != 2 {
		return types.ExecutionResult{
			RunIndex:      execSess.RunIndex,
			ExitCode:      exitCode,
			Duration:      duration,
			StderrSummary: truncate(stderrStr, 512),
			StateAtExit:   types.StateScenarioFailed,
		}, errors.NewExecutionError(fmt.Sprintf("subprocess exit %d: %s", exitCode, truncate(stderrStr, 256)), nil)
	}

	state := types.StateSubprocessExited
	if exitCode == 2 {
		state = types.StateSubprocessCrashed
	}

	return types.ExecutionResult{
		RunIndex:      execSess.RunIndex,
		ExitCode:      exitCode,
		Duration:      duration,
		StderrSummary: truncate(stderrStr, 512),
		StateAtExit:   state,
	}, nil
}

type limitedBuffer struct {
	buf   bytes.Buffer
	limit int
	trunc bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.trunc = true
		return len(p), nil
	}
	if len(p) > remaining {
		b.trunc = true
		_, _ = b.buf.Write(p[:remaining])
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *limitedBuffer) String() string {
	s := b.buf.String()
	if b.trunc {
		return s + "...(truncated)"
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
