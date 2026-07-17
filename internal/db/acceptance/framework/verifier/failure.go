package verifier

import "fmt"

// Severity classifies a verification failure.
type Severity string

const (
	// SeverityError is a hard invariant violation (fail the scenario).
	SeverityError Severity = "error"
	// SeverityWarning is a non-fatal anomaly recorded for diagnostics.
	SeverityWarning Severity = "warning"
)

// RecoveryPhase identifies which stage of recovery/verification produced a failure.
type RecoveryPhase string

const (
	// PhaseOracleLoad is oracle file load/validation.
	PhaseOracleLoad RecoveryPhase = "oracle_load"
	// PhaseDatabaseOpen is opening the recovered database directory.
	PhaseDatabaseOpen RecoveryPhase = "database_open"
	// PhaseVerify is module-level logical verification.
	PhaseVerify RecoveryPhase = "verify"
	// PhaseIdempotentReopen is repeated reopen verification.
	PhaseIdempotentReopen RecoveryPhase = "idempotent_reopen"
)

// Failure is a structured, human-readable invariant mismatch.
type Failure struct {
	Verifier       string        `json:"verifier"`
	Key            string        `json:"key,omitempty"`
	ExpectedValue  string        `json:"expected_value"`
	RecoveredValue string        `json:"recovered_value"`
	Reason         string        `json:"reason"`
	Severity       Severity      `json:"severity"`
	RecoveryPhase  RecoveryPhase `json:"recovery_phase"`
	Explanation    string        `json:"explanation"`
}

// String returns a single-line diagnostic suitable for logs.
func (f Failure) String() string {
	key := f.Key
	if key == "" {
		key = "-"
	}
	return fmt.Sprintf("%s/%s key=%s reason=%s expected=%s recovered=%s: %s",
		f.Verifier, f.RecoveryPhase, key, f.Reason, f.ExpectedValue, f.RecoveredValue, f.Explanation)
}

// newFailure builds an error-severity failure in the verify phase.
func newFailure(verifier, key, expected, recovered, reason, explanation string) Failure {
	return Failure{
		Verifier:       verifier,
		Key:            key,
		ExpectedValue:  expected,
		RecoveredValue: recovered,
		Reason:         reason,
		Severity:       SeverityError,
		RecoveryPhase:  PhaseVerify,
		Explanation:    explanation,
	}
}
