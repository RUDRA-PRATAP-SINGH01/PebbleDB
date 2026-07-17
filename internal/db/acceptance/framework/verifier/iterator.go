package verifier

import (
	"bytes"
	"fmt"
	"time"
)

const iteratorVerifierName = "iterator_verifier"

// IteratorVerifier validates forward iteration, Seek-based reverse order checks,
// boundary keys, duplicates, and iterator validity over the full keyspace.
//
// PebbleDB ScanIterator has no Prev(); reverse order is validated by seeking
// each live key from last to first and asserting positioning.
type IteratorVerifier struct{}

// Name returns the module identifier.
func (IteratorVerifier) Name() string { return iteratorVerifierName }

// Verify runs iterator invariants against the recovered database.
func (IteratorVerifier) Verify(vctx *VerificationContext) (*ModuleResult, error) {
	start := time.Now()
	res := emptyModuleResult(iteratorVerifierName)
	expected := vctx.Expected()
	database := vctx.Database()
	if expected == nil {
		return res, fmt.Errorf("iterator_verifier: nil expected state")
	}
	if database == nil {
		return res, fmt.Errorf("iterator_verifier: nil database")
	}

	live := expected.LiveKeys()

	it, err := database.Scan(nil, nil)
	if err != nil {
		return finalizeModule(res, start), fmt.Errorf("iterator_verifier: scan: %w", err)
	}
	defer it.Close()
	res.ScansPerformed++

	seen := make(map[string]struct{}, len(live))
	var prev []byte
	var gotKeys [][]byte
	for it.Valid() {
		if err := vctx.Err(); err != nil {
			return finalizeModule(res, start), err
		}
		k := append([]byte(nil), it.Key()...)
		ks := string(k)
		res.KeysVerified++

		if _, dup := seen[ks]; dup {
			res.Failures = append(res.Failures, newFailure(
				iteratorVerifierName, ks,
				"unique key", "duplicate",
				"duplicate_key",
				"Forward iterator emitted the same key more than once",
			))
		}
		seen[ks] = struct{}{}

		if prev != nil && bytes.Compare(prev, k) >= 0 {
			res.Failures = append(res.Failures, newFailure(
				iteratorVerifierName, ks,
				"strictly ascending order",
				fmt.Sprintf("prev=%q", prev),
				"ordering_violation",
				"Forward iterator keys are not in ascending order",
			))
		}
		prev = k
		gotKeys = append(gotKeys, k)

		snap, ok := expected.Get(k)
		if !ok || snap.Tombstone {
			res.Failures = append(res.Failures, newFailure(
				iteratorVerifierName, ks,
				"live oracle key",
				"unexpected or tombstone",
				"unexpected_key",
				"Iterator returned a key that is not a live oracle entry",
			))
		} else if !bytes.Equal(it.Value(), snap.Value) {
			res.Failures = append(res.Failures, newFailure(
				iteratorVerifierName, ks,
				formatBytes(snap.Value),
				formatBytes(it.Value()),
				"value_mismatch",
				"Iterator value does not match oracle",
			))
		} else {
			res.PassedChecks++
		}

		if err := it.Next(); err != nil {
			return finalizeModule(res, start), err
		}
	}

	if !it.Valid() {
		res.PassedChecks++ // exhausted cleanly
	}

	if len(gotKeys) != len(live) {
		res.Failures = append(res.Failures, newFailure(
			iteratorVerifierName, "",
			fmt.Sprintf("live_key_count=%d", len(live)),
			fmt.Sprintf("iterated=%d", len(gotKeys)),
			"key_count_mismatch",
			"Full-keyspace forward iteration count does not match oracle live keys",
		))
	}

	for _, key := range live {
		if _, ok := seen[string(key)]; !ok {
			res.Failures = append(res.Failures, newFailure(
				iteratorVerifierName, string(key),
				"present in forward scan",
				"missing",
				"missing_key",
				"Live oracle key was not visited by the forward iterator",
			))
		}
	}

	// Boundary: first and last live keys must match iterator endpoints when non-empty.
	if len(live) > 0 && len(gotKeys) > 0 {
		if !bytes.Equal(gotKeys[0], live[0]) {
			res.Failures = append(res.Failures, newFailure(
				iteratorVerifierName, string(gotKeys[0]),
				formatBytes(live[0]),
				formatBytes(gotKeys[0]),
				"boundary_first",
				"First iterated key does not match first live oracle key",
			))
		} else {
			res.PassedChecks++
		}
		if !bytes.Equal(gotKeys[len(gotKeys)-1], live[len(live)-1]) {
			res.Failures = append(res.Failures, newFailure(
				iteratorVerifierName, string(gotKeys[len(gotKeys)-1]),
				formatBytes(live[len(live)-1]),
				formatBytes(gotKeys[len(gotKeys)-1]),
				"boundary_last",
				"Last iterated key does not match last live oracle key",
			))
		} else {
			res.PassedChecks++
		}
	}

	// Reverse order via Seek (no Prev API).
	if len(live) > 0 {
		rev, err := database.Scan(nil, nil)
		if err != nil {
			return finalizeModule(res, start), fmt.Errorf("iterator_verifier: reverse scan: %w", err)
		}
		defer rev.Close()
		res.ScansPerformed++
		for i := len(live) - 1; i >= 0; i-- {
			if err := vctx.Err(); err != nil {
				return finalizeModule(res, start), err
			}
			want := live[i]
			if err := rev.Seek(want); err != nil {
				res.Failures = append(res.Failures, newFailure(
					iteratorVerifierName, string(want),
					"Seek succeeds",
					err.Error(),
					"seek_error",
					"Seek failed while validating reverse key order",
				))
				continue
			}
			if !rev.Valid() {
				res.Failures = append(res.Failures, newFailure(
					iteratorVerifierName, string(want),
					"Valid after Seek",
					"invalid",
					"seek_invalid",
					"Iterator not valid after Seek to live key (reverse check)",
				))
				continue
			}
			if !bytes.Equal(rev.Key(), want) {
				res.Failures = append(res.Failures, newFailure(
					iteratorVerifierName, string(want),
					formatBytes(want),
					formatBytes(rev.Key()),
					"seek_position",
					"Seek did not position on the requested live key during reverse validation",
				))
				continue
			}
			res.PassedChecks++
			res.KeysVerified++
		}
	}

	return finalizeModule(res, start), nil
}
