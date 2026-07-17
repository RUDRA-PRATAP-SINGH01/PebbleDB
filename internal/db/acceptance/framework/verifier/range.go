package verifier

import (
	"bytes"
	"fmt"
	"time"
)

const rangeScanVerifierName = "range_scan_verifier"

// RangeScanVerifier checks full, partial, and prefix scans against the oracle.
type RangeScanVerifier struct{}

// Name returns the module identifier.
func (RangeScanVerifier) Name() string { return rangeScanVerifierName }

// Verify runs range/prefix scan invariants.
func (RangeScanVerifier) Verify(vctx *VerificationContext) (*ModuleResult, error) {
	start := time.Now()
	res := emptyModuleResult(rangeScanVerifierName)
	expected := vctx.Expected()
	database := vctx.Database()
	if expected == nil {
		return res, fmt.Errorf("range_scan_verifier: nil expected state")
	}
	if database == nil {
		return res, fmt.Errorf("range_scan_verifier: nil database")
	}

	live := expected.LiveKeys()

	if err := verifyRange(vctx, res, nil, nil, live, "full_scan"); err != nil {
		return finalizeModule(res, start), err
	}

	if len(live) >= 2 {
		mid := live[len(live)/2]
		end := live[len(live)-1]
		// Partial half-open [mid, end) — excludes last key.
		wantPartial := expectedInRange(expected, mid, end)
		if err := verifyRange(vctx, res, mid, end, wantPartial, "partial_scan"); err != nil {
			return finalizeModule(res, start), err
		}

		// Inclusive-style check: [first, after-last) via end=nil from start.
		wantTail := expectedInRange(expected, mid, nil)
		if err := verifyRange(vctx, res, mid, nil, wantTail, "tail_scan"); err != nil {
			return finalizeModule(res, start), err
		}
	}

	// Prefix scan using sequential generator key prefix "key_".
	prefix := []byte("key_")
	prefixEnd := nextPrefixBound(prefix)
	wantPrefix := expectedInRange(expected, prefix, prefixEnd)
	if err := verifyRange(vctx, res, prefix, prefixEnd, wantPrefix, "prefix_scan"); err != nil {
		return finalizeModule(res, start), err
	}

	return finalizeModule(res, start), nil
}

func verifyRange(
	vctx *VerificationContext,
	res *ModuleResult,
	start, end []byte,
	want [][]byte,
	label string,
) error {
	it, err := vctx.Database().Scan(start, end)
	if err != nil {
		return fmt.Errorf("range_scan_verifier: %s: %w", label, err)
	}
	defer it.Close()
	res.ScansPerformed++

	gotKeys, gotVals, err := collectLiveScan(it)
	if err != nil {
		return err
	}

	if len(gotKeys) != len(want) {
		res.Failures = append(res.Failures, newFailure(
			rangeScanVerifierName, label,
			fmt.Sprintf("count=%d", len(want)),
			fmt.Sprintf("count=%d", len(gotKeys)),
			"range_count_mismatch",
			fmt.Sprintf("Scan %s returned unexpected key count for range [%q,%q)", label, start, end),
		))
	}

	n := len(gotKeys)
	if len(want) < n {
		n = len(want)
	}
	var prev []byte
	for i := 0; i < n; i++ {
		if err := vctx.Err(); err != nil {
			return err
		}
		res.KeysVerified++
		if !bytes.Equal(gotKeys[i], want[i]) {
			res.Failures = append(res.Failures, newFailure(
				rangeScanVerifierName, string(gotKeys[i]),
				formatBytes(want[i]),
				formatBytes(gotKeys[i]),
				"range_key_mismatch",
				fmt.Sprintf("Scan %s key at index %d does not match oracle ordering", label, i),
			))
			continue
		}
		snap, ok := vctx.Expected().Get(gotKeys[i])
		if !ok || snap.Tombstone {
			res.Failures = append(res.Failures, newFailure(
				rangeScanVerifierName, string(gotKeys[i]),
				"live oracle value",
				"missing/tombstone",
				"unexpected_key",
				fmt.Sprintf("Scan %s returned non-live key", label),
			))
			continue
		}
		if !bytes.Equal(gotVals[i], snap.Value) {
			res.Failures = append(res.Failures, newFailure(
				rangeScanVerifierName, string(gotKeys[i]),
				formatBytes(snap.Value),
				formatBytes(gotVals[i]),
				"value_mismatch",
				fmt.Sprintf("Scan %s value mismatch", label),
			))
			continue
		}
		if prev != nil && bytes.Compare(prev, gotKeys[i]) >= 0 {
			res.Failures = append(res.Failures, newFailure(
				rangeScanVerifierName, string(gotKeys[i]),
				"ascending order",
				fmt.Sprintf("prev=%q", prev),
				"ordering_violation",
				fmt.Sprintf("Scan %s keys are not ordered", label),
			))
		} else {
			res.PassedChecks++
		}
		prev = gotKeys[i]
	}

	// Boundary: first/last of range when non-empty.
	if len(want) > 0 && len(gotKeys) > 0 {
		if bytes.Equal(gotKeys[0], want[0]) && bytes.Equal(gotKeys[len(gotKeys)-1], want[len(want)-1]) {
			res.PassedChecks++
		}
	} else if len(want) == 0 && len(gotKeys) == 0 {
		res.PassedChecks++
	}
	return nil
}
