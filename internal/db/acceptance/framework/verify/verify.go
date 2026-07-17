// Package verify implements post-recovery logical invariant checks for ATF.
package verify

import (
	"bytes"
	"context"
	"fmt"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/dataset"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// GetVerifier checks every expected key via Get after recovery.
type GetVerifier struct{}

func (GetVerifier) Name() string { return "get_verifier" }

// Verify opens nothing — caller supplies an already-open DB and expected state.
func (GetVerifier) Verify(ctx context.Context, database *db.DB, expected *dataset.MapExpectedState) (*types.VerificationResult, error) {
	res := &types.VerificationResult{
		Passed:  true,
		Timings: make(map[string]float64),
	}
	if expected == nil {
		return res, fmt.Errorf("verify: nil expected state")
	}

	for _, key := range expected.Keys() {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		snap, ok := expected.Get(key)
		if !ok {
			continue
		}
		val, err := database.Get(key)
		if snap.Tombstone {
			if err == nil {
				res.Passed = false
				res.Failures = append(res.Failures, types.VerifierFailure{
					Verifier: "get_verifier",
					Key:      string(key),
					Expected: "ErrNotFound (tombstone)",
					Actual:   fmt.Sprintf("value=%q", val),
					Details:  "deleted key still visible after recovery",
				})
			} else if err != db.ErrNotFound {
				res.Passed = false
				res.Failures = append(res.Failures, types.VerifierFailure{
					Verifier: "get_verifier",
					Key:      string(key),
					Expected: "ErrNotFound",
					Actual:   err.Error(),
				})
			}
			continue
		}
		if err != nil {
			res.Passed = false
			res.Failures = append(res.Failures, types.VerifierFailure{
				Verifier: "get_verifier",
				Key:      string(key),
				Expected: fmt.Sprintf("%q", snap.Value),
				Actual:   err.Error(),
				Details:  "live key missing after recovery",
			})
			continue
		}
		if !bytes.Equal(val, snap.Value) {
			res.Passed = false
			res.Failures = append(res.Failures, types.VerifierFailure{
				Verifier: "get_verifier",
				Key:      string(key),
				Expected: fmt.Sprintf("%q", snap.Value),
				Actual:   fmt.Sprintf("%q", val),
				Details:  "value mismatch after recovery",
			})
		}
	}
	return res, nil
}

// ScanVerifier ensures a full scan returns live keys in order without tombstones/duplicates.
type ScanVerifier struct{}

func (ScanVerifier) Name() string { return "scan_verifier" }

func (ScanVerifier) Verify(ctx context.Context, database *db.DB, expected *dataset.MapExpectedState) (*types.VerificationResult, error) {
	res := &types.VerificationResult{Passed: true, Timings: make(map[string]float64)}
	it, err := database.Scan(nil, nil)
	if err != nil {
		return res, err
	}
	defer it.Close()

	seen := make(map[string]struct{})
	var prev []byte
	for it.Valid() {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		k := append([]byte(nil), it.Key()...)
		ks := string(k)
		if _, dup := seen[ks]; dup {
			res.Passed = false
			res.Failures = append(res.Failures, types.VerifierFailure{
				Verifier: "scan_verifier",
				Key:      ks,
				Expected: "unique key",
				Actual:   "duplicate",
			})
		}
		seen[ks] = struct{}{}
		if prev != nil && bytes.Compare(prev, k) >= 0 {
			res.Passed = false
			res.Failures = append(res.Failures, types.VerifierFailure{
				Verifier: "scan_verifier",
				Key:      ks,
				Expected: "ascending order",
				Actual:   fmt.Sprintf("prev=%q", prev),
			})
		}
		snap, ok := expected.Get(k)
		if !ok || snap.Tombstone {
			res.Passed = false
			res.Failures = append(res.Failures, types.VerifierFailure{
				Verifier: "scan_verifier",
				Key:      ks,
				Expected: "absent or live only",
				Actual:   "unexpected live key in scan",
			})
		} else if !bytes.Equal(it.Value(), snap.Value) {
			res.Passed = false
			res.Failures = append(res.Failures, types.VerifierFailure{
				Verifier: "scan_verifier",
				Key:      ks,
				Expected: fmt.Sprintf("%q", snap.Value),
				Actual:   fmt.Sprintf("%q", it.Value()),
			})
		}
		prev = k
		if err := it.Next(); err != nil {
			return res, err
		}
	}

	for _, key := range expected.Keys() {
		snap, _ := expected.Get(key)
		if snap.Tombstone {
			continue
		}
		if _, ok := seen[string(key)]; !ok {
			res.Passed = false
			res.Failures = append(res.Failures, types.VerifierFailure{
				Verifier: "scan_verifier",
				Key:      string(key),
				Expected: "present in scan",
				Actual:   "missing",
			})
		}
	}
	return res, nil
}
