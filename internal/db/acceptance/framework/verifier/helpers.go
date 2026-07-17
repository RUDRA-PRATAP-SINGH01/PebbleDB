package verifier

import (
	"bytes"
	"fmt"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/dataset"
)

func emptyModuleResult(name string) *ModuleResult {
	return &ModuleResult{
		Name:   name,
		Passed: true,
	}
}

func finalizeModule(res *ModuleResult, start time.Time) *ModuleResult {
	res.Duration = time.Since(start)
	res.FailedChecks = len(res.Failures)
	if res.FailedChecks > 0 {
		res.Passed = false
	}
	for _, f := range res.Failures {
		if f.Severity == SeverityWarning {
			res.Warnings++
		}
	}
	return res
}

func collectLiveScan(it *db.ScanIterator) ([][]byte, [][]byte, error) {
	var keys, vals [][]byte
	for it.Valid() {
		keys = append(keys, append([]byte(nil), it.Key()...))
		vals = append(vals, append([]byte(nil), it.Value()...))
		if err := it.Next(); err != nil {
			return nil, nil, err
		}
	}
	return keys, vals, nil
}

func expectedInRange(expected *dataset.MapExpectedState, start, end []byte) [][]byte {
	live := expected.LiveKeys()
	out := make([][]byte, 0, len(live))
	for _, k := range live {
		if len(start) > 0 && bytes.Compare(k, start) < 0 {
			continue
		}
		if len(end) > 0 && bytes.Compare(k, end) >= 0 {
			continue
		}
		out = append(out, k)
	}
	return out
}

func formatBytes(b []byte) string {
	if b == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%q", b)
}

func nextPrefixBound(prefix []byte) []byte {
	out := append([]byte(nil), prefix...)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i] < 0xff {
			out[i]++
			return out[:i+1]
		}
	}
	return nil // no finite upper bound
}

func countTombstones(expected *dataset.MapExpectedState) int {
	n := 0
	for _, snap := range expected.State {
		if snap.Tombstone {
			n++
		}
	}
	return n
}
