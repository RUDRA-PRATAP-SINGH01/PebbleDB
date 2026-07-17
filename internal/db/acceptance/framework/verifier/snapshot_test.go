package verifier

import "testing"

func TestSnapshotVerifierPass(t *testing.T) {
	h := openHarness(t, 41, 35, 6, 5)
	res, err := (SnapshotVerifier{}).Verify(testContext(h))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatalf("failures: %+v", res.Failures)
	}
}

func TestSnapshotVerifierTombstoneNotVisible(t *testing.T) {
	h := openHarness(t, 42, 20, 0, 4)
	res, err := (SnapshotVerifier{}).Verify(testContext(h))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatalf("tombstones should be hidden: %+v", res.Failures)
	}
}
