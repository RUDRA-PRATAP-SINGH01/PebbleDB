package verifier

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/dataset"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/errors"
)

// OracleLoadOptions controls optional identity checks against the oracle file.
type OracleLoadOptions struct {
	// RequireChecksum rejects oracles without a checksum (production default: true).
	RequireChecksum bool
	// ExpectedScenarioID when non-empty must match the oracle scenario_id.
	ExpectedScenarioID string
	// ExpectedExecutionID when non-empty must match the oracle execution_id.
	ExpectedExecutionID string
}

// OracleLoader loads and validates expected_state.json for verification.
type OracleLoader struct {
	opts OracleLoadOptions
}

// NewOracleLoader constructs a loader with the given options.
func NewOracleLoader(opts OracleLoadOptions) *OracleLoader {
	return &OracleLoader{opts: opts}
}

// DefaultOracleLoader returns a production loader that requires checksums.
func DefaultOracleLoader() *OracleLoader {
	return NewOracleLoader(OracleLoadOptions{RequireChecksum: true})
}

// Load reads dir/expected_state.json, validates schema and checksum, and returns state.
func (l *OracleLoader) Load(dir string) (*dataset.MapExpectedState, error) {
	path := filepath.Join(dir, dataset.ExpectedStateFileName)
	if _, err := os.Stat(path); err != nil {
		return nil, errors.NewValidationError("oracle file missing", err)
	}

	state, err := dataset.LoadExpectedState(dir)
	if err != nil {
		return nil, errors.NewValidationError("oracle unmarshal failed", err)
	}

	if err := l.validate(state); err != nil {
		return nil, err
	}
	return state, nil
}

func (l *OracleLoader) validate(state *dataset.MapExpectedState) error {
	if state == nil {
		return errors.NewValidationError("oracle is nil", nil)
	}
	if state.SchemaVersion != dataset.OracleSchemaVersion {
		return errors.NewValidationError(
			fmt.Sprintf("unsupported oracle schema_version=%d want=%d", state.SchemaVersion, dataset.OracleSchemaVersion),
			nil,
		)
	}
	if state.State == nil {
		return errors.NewValidationError("oracle state map is nil", nil)
	}
	if state.Count < 0 {
		return errors.NewValidationError("oracle count is negative", nil)
	}
	if state.Count > 0 && len(state.State) == 0 {
		return errors.NewValidationError("oracle count>0 but state is empty", nil)
	}

	if l.opts.RequireChecksum || state.Checksum != "" {
		if state.Checksum == "" {
			return errors.NewValidationError("oracle checksum missing", nil)
		}
		got, err := state.ComputeChecksum()
		if err != nil {
			return errors.NewValidationError("oracle checksum compute failed", err)
		}
		if got != state.Checksum {
			return errors.NewValidationError(
				fmt.Sprintf("oracle checksum mismatch got=%s want=%s", got, state.Checksum),
				nil,
			)
		}
	}

	if l.opts.ExpectedScenarioID != "" {
		if state.ScenarioID == "" {
			return errors.NewValidationError("oracle scenario_id missing", nil)
		}
		if state.ScenarioID != l.opts.ExpectedScenarioID {
			return errors.NewValidationError(
				fmt.Sprintf("oracle scenario_id mismatch got=%q want=%q", state.ScenarioID, l.opts.ExpectedScenarioID),
				nil,
			)
		}
	}
	if l.opts.ExpectedExecutionID != "" {
		if state.ExecutionID == "" {
			return errors.NewValidationError("oracle execution_id missing", nil)
		}
		if state.ExecutionID != l.opts.ExpectedExecutionID {
			return errors.NewValidationError(
				fmt.Sprintf("oracle execution_id mismatch got=%q want=%q", state.ExecutionID, l.opts.ExpectedExecutionID),
				nil,
			)
		}
	}

	tombstones := 0
	for _, snap := range state.State {
		if snap.Tombstone {
			tombstones++
		}
	}
	_ = tombstones // validated structurally; stats computed by engine
	return nil
}
