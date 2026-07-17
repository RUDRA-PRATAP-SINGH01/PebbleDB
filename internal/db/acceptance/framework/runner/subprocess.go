// Package runner orchestrates subprocess execution campaigns.
package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/errors"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/interfaces"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// SubprocessController spawns, manages, and terminates child processes under isolation.
type SubprocessController struct {
	logger  *logging.Logger
	timeout time.Duration
}

// NewSubprocessController allocates a new SubprocessController.
func NewSubprocessController(logger *logging.Logger, timeout time.Duration) *SubprocessController {
	return &SubprocessController{
		logger:  logger,
		timeout: timeout,
	}
}

// RunSubprocess executes the scenario payload inside a child process, capturing stdout/stderr and exit codes.
func (s *SubprocessController) RunSubprocess(
	ctx context.Context,
	execSess types.ExecutionSession,
	scenario interfaces.Scenario,
) (types.ExecutionResult, error) {
	// Build target test command re-invoking the test binary itself
	cmd := exec.Command(os.Args[0], "-test.run=^TestFramework", "-test.count=1")

	// Assemble Environment variables mapping the Child Process Protocol
	cmd.Env = append(os.Environ(),
		"PEBBLEDB_CHILD_PROCESS=1",
		"PEBBLEDB_SCENARIO_ID="+scenario.ID(),
		"PEBBLEDB_TEST_DIR="+execSess.TempDir,
		"PEBBLEDB_CRASH_AT="+scenario.CrashPoint(),
	)

	// Append database config overrides from options if present
	opts := scenario.Options()
	if val, ok := opts["memtable_size_bytes"]; ok {
		cmd.Env = append(cmd.Env, fmt.Sprintf("PEBBLEDB_MEMTABLE_SIZE=%v", val))
	}
	if val, ok := opts["compaction_threshold"]; ok {
		cmd.Env = append(cmd.Env, fmt.Sprintf("PEBBLEDB_COMPACTION_THRESHOLD=%v", val))
	}
	if val, ok := opts["sync_writes"]; ok {
		cmd.Env = append(cmd.Env, fmt.Sprintf("PEBBLEDB_SYNC_WRITES=%v", val))
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	s.logger.Debug("Spawning child process for scenario %s (TempDir: %s)", scenario.ID(), execSess.TempDir)

	if err := cmd.Start(); err != nil {
		return types.ExecutionResult{}, errors.NewExecutionError("failed to start subprocess", err)
	}

	// Channel to signal process termination
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	startTime := time.Now()
	var exitCode int
	var err error

	select {
	case err = <-done:
		// Process exited
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
					exitCode = status.ExitStatus()
				} else {
					exitCode = exitErr.ExitCode()
				}
			} else {
				exitCode = 1
			}
		} else {
			exitCode = 0
		}
	case <-ctx.Done():
		// Context cancellation or timeout exceeded
		s.logger.Warn("Subprocess for scenario %s timed out or canceled. Terminating process ID: %d", scenario.ID(), cmd.Process.Pid)
		_ = cmd.Process.Kill()
		<-done // wait for clean release
		return types.ExecutionResult{
			RunIndex:      execSess.RunIndex,
			ExitCode:      -1,
			Duration:      float64(time.Since(startTime).Milliseconds()),
			StderrSummary: "terminated by context cancel / timeout",
			StateAtExit:   types.StateScenarioFailed,
		}, ctx.Err()
	}

	duration := float64(time.Since(startTime).Milliseconds())
	stderrStr := stderrBuf.String()

	s.logger.Debug("Subprocess exited for scenario %s: exitCode=%d, duration=%.2fms", scenario.ID(), exitCode, duration)

	if exitCode != 0 && exitCode != 2 {
		// Exit code 2 is expected for deterministic crash points in future sprints.
		// Other non-zero exit codes represent errors.
		return types.ExecutionResult{
			RunIndex:      execSess.RunIndex,
			ExitCode:      exitCode,
			Duration:      duration,
			StderrSummary: stderrStr,
			StateAtExit:   types.StateScenarioFailed,
		}, errors.NewExecutionError(fmt.Sprintf("subprocess failed with exit code %d. Stderr: %q", exitCode, stderrStr), nil)
	}

	return types.ExecutionResult{
		RunIndex:      execSess.RunIndex,
		ExitCode:      exitCode,
		Duration:      duration,
		StderrSummary: stderrStr,
		StateAtExit:   types.StateScenarioCompleted,
	}, nil
}
