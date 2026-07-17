package verifier

import (
	"bytes"
	"fmt"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db"
)

const getVerifierName = "get_verifier"

// GetVerifier checks point lookups against the oracle for every expected key.
type GetVerifier struct{}

// Name returns the module identifier.
func (GetVerifier) Name() string { return getVerifierName }

// Verify asserts live keys exist with exact values and tombstones are absent.
func (GetVerifier) Verify(vctx *VerificationContext) (*ModuleResult, error) {
	start := time.Now()
	res := emptyModuleResult(getVerifierName)
	expected := vctx.Expected()
	database := vctx.Database()
	if expected == nil {
		return res, fmt.Errorf("get_verifier: nil expected state")
	}
	if database == nil {
		return res, fmt.Errorf("get_verifier: nil database")
	}

	for _, key := range expected.Keys() {
		if err := vctx.Err(); err != nil {
			return finalizeModule(res, start), err
		}
		snap, ok := expected.Get(key)
		if !ok {
			continue
		}
		res.KeysVerified++
		val, err := database.Get(key)
		ks := string(key)

		if snap.Tombstone {
			if err == nil {
				res.Failures = append(res.Failures, newFailure(
					getVerifierName, ks,
					"ErrNotFound (tombstone)",
					formatBytes(val),
					"deleted_key_present",
					"Deleted key is still visible after recovery via Get()",
				))
			} else if err != db.ErrNotFound {
				res.Failures = append(res.Failures, newFailure(
					getVerifierName, ks,
					"ErrNotFound",
					err.Error(),
					"unexpected_get_error",
					"Tombstone key returned an unexpected error instead of ErrNotFound",
				))
			} else {
				res.PassedChecks++
			}
			continue
		}

		if err != nil {
			res.Failures = append(res.Failures, newFailure(
				getVerifierName, ks,
				formatBytes(snap.Value),
				err.Error(),
				"missing_key",
				"Live key is missing after recovery",
			))
			continue
		}
		if !bytes.Equal(val, snap.Value) {
			res.Failures = append(res.Failures, newFailure(
				getVerifierName, ks,
				formatBytes(snap.Value),
				formatBytes(val),
				"value_mismatch",
				"Recovered value does not match oracle value",
			))
			continue
		}
		res.PassedChecks++
	}
	return finalizeModule(res, start), nil
}
