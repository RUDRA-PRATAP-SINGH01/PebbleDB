package verifier

import (
	"fmt"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db"
)

const metadataVerifierName = "metadata_verifier"

// MetadataVerifier checks that the recovered database opened cleanly and that
// global recovery invariants hold before the per-key modules run: no background
// error, oracle metadata is well-formed, and the recovered live-key population
// matches the oracle's live count. It is intended to gate the heavier modules so
// a fundamentally broken recovery is caught early.
type MetadataVerifier struct{}

// Name returns the module identifier.
func (MetadataVerifier) Name() string { return metadataVerifierName }

// Verify inspects open health, oracle metadata, and the recovered live-key count.
func (MetadataVerifier) Verify(vctx *VerificationContext) (*ModuleResult, error) {
	start := time.Now()
	res := emptyModuleResult(metadataVerifierName)
	database := vctx.Database()
	if database == nil {
		return res, fmt.Errorf("metadata_verifier: nil database")
	}
	expected := vctx.Expected()
	if expected == nil {
		return res, fmt.Errorf("metadata_verifier: nil expected state")
	}

	report := vctx.Report()
	if report != nil {
		report.DatabaseMetadata.OpenedCleanly = true
		report.DatabaseMetadata.Dir = vctx.DatabasePath()
		report.DatabaseMetadata.CompactionDisabled = true
		report.ExpectedStatistics = Statistics{
			ExpectedLiveKeys:   expected.LiveCount(),
			ExpectedTombstones: countTombstones(expected),
			OracleSeed:         expected.Seed,
			OracleCount:        expected.Count,
		}
	}

	// 1. No background flush/compaction/WAL error after recovery.
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

	// 2. Oracle metadata must be internally coherent.
	if expected.SchemaVersion <= 0 {
		res.Failures = append(res.Failures, newFailure(
			metadataVerifierName, "",
			"schema_version > 0", fmt.Sprintf("%d", expected.SchemaVersion),
			"bad_oracle_schema", "Oracle schema version is missing or invalid"))
	} else {
		res.PassedChecks++
	}
	if expected.LiveCount()+countTombstones(expected) != len(expected.State) {
		res.Failures = append(res.Failures, newFailure(
			metadataVerifierName, "",
			fmt.Sprintf("live+tombstone == %d", len(expected.State)),
			fmt.Sprintf("live=%d tombstone=%d", expected.LiveCount(), countTombstones(expected)),
			"oracle_partition_mismatch",
			"Oracle live and tombstone partitions do not sum to the total key count"))
	} else {
		res.PassedChecks++
	}

	// 3. Recovered live population must match the oracle live count exactly.
	recoveredLive, err := metadataCountLive(vctx, database)
	if err != nil {
		return finalizeModule(res, start), err
	}
	if report != nil {
		report.RecoveredStatistics = Statistics{
			ExpectedLiveKeys:   expected.LiveCount(),
			ExpectedTombstones: countTombstones(expected),
			RecoveredLiveKeys:  recoveredLive,
			OracleSeed:         expected.Seed,
			OracleCount:        expected.Count,
		}
	}
	if recoveredLive != expected.LiveCount() {
		res.Failures = append(res.Failures, newFailure(
			metadataVerifierName, "",
			fmt.Sprintf("recovered_live=%d", expected.LiveCount()),
			fmt.Sprintf("recovered_live=%d", recoveredLive),
			"live_count_mismatch",
			"Recovered live-key count does not match the oracle after recovery"))
	} else {
		res.PassedChecks++
	}

	// 4. Empty-key probe must not corrupt state: it must return ErrNotFound
	//    (empty key is never written by the generator) rather than a value.
	if _, gErr := database.Get([]byte{}); gErr == db.ErrNotFound {
		res.PassedChecks++
	} else if gErr == nil {
		res.Failures = append(res.Failures, newFailure(
			metadataVerifierName, "",
			"ErrNotFound for empty key", "value returned",
			"unexpected_empty_key", "Empty key unexpectedly resolved to a value after recovery"))
	} else {
		// Any other error is a real fault surfaced by the point lookup path.
		res.Failures = append(res.Failures, newFailure(
			metadataVerifierName, "",
			"ErrNotFound for empty key", gErr.Error(),
			"empty_key_error", "Empty-key lookup returned an unexpected error"))
	}

	return finalizeModule(res, start), nil
}

func metadataCountLive(vctx *VerificationContext, database *db.DB) (int, error) {
	it, err := database.Scan(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("metadata_verifier: scan: %w", err)
	}
	defer it.Close()
	n := 0
	for it.Valid() {
		if err := vctx.Err(); err != nil {
			return n, err
		}
		n++
		if err := it.Next(); err != nil {
			return n, err
		}
	}
	return n, nil
}
