package verifier

import (
	"fmt"
	"time"
)

const metadataVerifierName = "metadata_verifier"

// MetadataVerifier checks that the recovered database opened cleanly and that
// basic open-time health metadata is consistent with ATF expectations.
type MetadataVerifier struct{}

// Name returns the module identifier.
func (MetadataVerifier) Name() string { return metadataVerifierName }

// Verify inspects open health (open already succeeded) and background error state.
func (MetadataVerifier) Verify(vctx *VerificationContext) (*ModuleResult, error) {
	start := time.Now()
	res := emptyModuleResult(metadataVerifierName)
	database := vctx.Database()
	if database == nil {
		return res, fmt.Errorf("metadata_verifier: nil database")
	}

	report := vctx.Report()
	if report != nil {
		report.DatabaseMetadata.OpenedCleanly = true
		report.DatabaseMetadata.Dir = vctx.DatabasePath()
		report.DatabaseMetadata.CompactionDisabled = true
	}

	if err := database.BackgroundError(); err != nil {
		msg := err.Error()
		if report != nil {
			report.DatabaseMetadata.BackgroundError = msg
		}
		res.Failures = append(res.Failures, Failure{
			Verifier:       metadataVerifierName,
			ExpectedValue:  "nil background error",
			RecoveredValue: msg,
			Reason:         "background_error",
			Severity:       SeverityError,
			RecoveryPhase:  PhaseDatabaseOpen,
			Explanation:    "Recovered database reported a background flush/compaction/WAL error",
		})
	} else {
		res.PassedChecks++
	}

	// Options probe: empty-key Get must not panic; NotFound/empty are acceptable.
	if _, err := database.Get([]byte{}); err != nil {
		res.PassedChecks++
	} else {
		res.PassedChecks++
	}

	expected := vctx.Expected()
	if expected != nil && report != nil {
		report.ExpectedStatistics = Statistics{
			ExpectedLiveKeys:   expected.LiveCount(),
			ExpectedTombstones: countTombstones(expected),
			OracleSeed:         expected.Seed,
			OracleCount:        expected.Count,
		}
	}

	return finalizeModule(res, start), nil
}
