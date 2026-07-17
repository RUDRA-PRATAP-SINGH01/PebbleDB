package verifier

import "time"

// ModuleResult is the outcome of a single Verifier module.
type ModuleResult struct {
	Name           string        `json:"name"`
	Passed         bool          `json:"passed"`
	Failures       []Failure     `json:"failures,omitempty"`
	PassedChecks   int           `json:"passed_checks"`
	FailedChecks   int           `json:"failed_checks"`
	Duration       time.Duration `json:"duration"`
	KeysVerified   int64         `json:"keys_verified"`
	ScansPerformed int64         `json:"scans_performed"`
	Warnings       int           `json:"warnings"`
}

// DatabaseMetadata summarizes recovered database open health.
type DatabaseMetadata struct {
	OpenedCleanly      bool   `json:"opened_cleanly"`
	BackgroundError    string `json:"background_error,omitempty"`
	CompactionDisabled bool   `json:"compaction_disabled"`
	Dir                string `json:"dir"`
}

// Statistics compares expected vs recovered live-key counts.
type Statistics struct {
	ExpectedLiveKeys   int   `json:"expected_live_keys"`
	ExpectedTombstones int   `json:"expected_tombstones"`
	RecoveredLiveKeys  int   `json:"recovered_live_keys"`
	OracleSeed         int64 `json:"oracle_seed"`
	OracleCount        int   `json:"oracle_count"`
}

// VerificationReport aggregates all module results for one execution.
type VerificationReport struct {
	ScenarioID          string           `json:"scenario_id"`
	ExecutionID         string           `json:"execution_id"`
	Passed              bool             `json:"passed"`
	ModuleResults       []ModuleResult   `json:"module_results"`
	PassedChecks        int              `json:"passed_checks"`
	FailedChecks        int              `json:"failed_checks"`
	Duration            time.Duration    `json:"duration"`
	DatabaseOpenTime    time.Duration    `json:"database_open_time"`
	DatabaseMetadata    DatabaseMetadata `json:"database_metadata"`
	ExpectedStatistics  Statistics       `json:"expected_statistics"`
	RecoveredStatistics Statistics       `json:"recovered_statistics"`
	FailureSummary      []string         `json:"failure_summary,omitempty"`
	Aborted             bool             `json:"aborted"`
	AbortReason         string           `json:"abort_reason,omitempty"`
}

// Failures flattens all module failures.
func (r *VerificationReport) Failures() []Failure {
	var out []Failure
	for _, m := range r.ModuleResults {
		out = append(out, m.Failures...)
	}
	return out
}

func (r *VerificationReport) addModule(m ModuleResult) {
	r.ModuleResults = append(r.ModuleResults, m)
	r.PassedChecks += m.PassedChecks
	r.FailedChecks += m.FailedChecks
	if !m.Passed {
		r.Passed = false
		for _, f := range m.Failures {
			r.FailureSummary = append(r.FailureSummary, f.String())
		}
	}
}
