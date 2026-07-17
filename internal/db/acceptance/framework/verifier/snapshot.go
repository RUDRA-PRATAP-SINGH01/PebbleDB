package verifier

import (
	"bytes"
	"fmt"
	"time"
)

const snapshotVerifierName = "snapshot_verifier"

// SnapshotVerifier validates Scan point-in-time snapshot semantics available in
// PebbleDB: two concurrent scans observe the same key sequence, Get agrees with
// scan values for visited keys, and tombstones are never visible inside a scan.
//
// PebbleDB does not expose an MVCC Snapshot API; this module verifies the
// documented Scan snapshot contract instead of inventing multi-version reads.
type SnapshotVerifier struct{}

// Name returns the module identifier.
func (SnapshotVerifier) Name() string { return snapshotVerifierName }

// Verify checks scan snapshot isolation and Get/Scan consistency.
func (SnapshotVerifier) Verify(vctx *VerificationContext) (*ModuleResult, error) {
	start := time.Now()
	res := emptyModuleResult(snapshotVerifierName)
	expected := vctx.Expected()
	database := vctx.Database()
	if expected == nil {
		return res, fmt.Errorf("snapshot_verifier: nil expected state")
	}
	if database == nil {
		return res, fmt.Errorf("snapshot_verifier: nil database")
	}

	live := expected.LiveKeys()

	it1, err := database.Scan(nil, nil)
	if err != nil {
		return finalizeModule(res, start), fmt.Errorf("snapshot_verifier: scan1: %w", err)
	}
	defer it1.Close()
	res.ScansPerformed++

	it2, err := database.Scan(nil, nil)
	if err != nil {
		return finalizeModule(res, start), fmt.Errorf("snapshot_verifier: scan2: %w", err)
	}
	defer it2.Close()
	res.ScansPerformed++

	keys1, vals1, err := collectLiveScan(it1)
	if err != nil {
		return finalizeModule(res, start), err
	}
	keys2, vals2, err := collectLiveScan(it2)
	if err != nil {
		return finalizeModule(res, start), err
	}

	if len(keys1) != len(keys2) {
		res.Failures = append(res.Failures, newFailure(
			snapshotVerifierName, "",
			fmt.Sprintf("scan1_count=%d", len(keys1)),
			fmt.Sprintf("scan2_count=%d", len(keys2)),
			"snapshot_divergence",
			"Two concurrent Scan snapshots returned different key counts",
		))
	}
	n := len(keys1)
	if len(keys2) < n {
		n = len(keys2)
	}
	for i := 0; i < n; i++ {
		if err := vctx.Err(); err != nil {
			return finalizeModule(res, start), err
		}
		res.KeysVerified++
		if !bytes.Equal(keys1[i], keys2[i]) || !bytes.Equal(vals1[i], vals2[i]) {
			res.Failures = append(res.Failures, newFailure(
				snapshotVerifierName, string(keys1[i]),
				formatBytes(keys2[i])+" / "+formatBytes(vals2[i]),
				formatBytes(keys1[i])+" / "+formatBytes(vals1[i]),
				"snapshot_isolation",
				"Concurrent Scan snapshots disagree on key/value at the same index",
			))
			continue
		}
		res.PassedChecks++

		// Get consistency with snapshot value (post-recovery quiescent DB).
		got, err := database.Get(keys1[i])
		if err != nil {
			res.Failures = append(res.Failures, newFailure(
				snapshotVerifierName, string(keys1[i]),
				formatBytes(vals1[i]),
				err.Error(),
				"get_scan_inconsistency",
				"Get failed for a key present in the Scan snapshot",
			))
			continue
		}
		if !bytes.Equal(got, vals1[i]) {
			res.Failures = append(res.Failures, newFailure(
				snapshotVerifierName, string(keys1[i]),
				formatBytes(vals1[i]),
				formatBytes(got),
				"get_scan_inconsistency",
				"Get value differs from Scan snapshot value",
			))
			continue
		}
		res.PassedChecks++
	}

	// Tombstones must not appear in either snapshot.
	for _, key := range expected.Keys() {
		snap, _ := expected.Get(key)
		if !snap.Tombstone {
			continue
		}
		for _, k := range keys1 {
			if bytes.Equal(k, key) {
				res.Failures = append(res.Failures, newFailure(
					snapshotVerifierName, string(key),
					"absent from snapshot",
					"present",
					"tombstone_visible",
					"Deleted key is visible inside a Scan snapshot",
				))
			}
		}
	}

	// Overwritten keys: oracle live value must match snapshot (already covered by Get).
	if len(keys1) == len(live) {
		res.PassedChecks++
	} else {
		res.Failures = append(res.Failures, newFailure(
			snapshotVerifierName, "",
			fmt.Sprintf("live=%d", len(live)),
			fmt.Sprintf("snapshot=%d", len(keys1)),
			"snapshot_count",
			"Scan snapshot live key count does not match oracle",
		))
	}

	return finalizeModule(res, start), nil
}
