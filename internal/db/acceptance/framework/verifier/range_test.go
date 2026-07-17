package verifier

import "testing"

func TestRangeScanVerifierPass(t *testing.T) {
	h := openHarness(t, 31, 40, 4, 5)
	res, err := (RangeScanVerifier{}).Verify(testContext(h))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatalf("failures: %+v", res.Failures)
	}
	if res.ScansPerformed < 3 {
		t.Fatalf("expected multiple scans, got %d", res.ScansPerformed)
	}
}

func TestRangeScanVerifierDetectsMissing(t *testing.T) {
	h := openHarness(t, 32, 25, 0, 0)
	live := h.expected.LiveKeys()
	if err := h.database.Delete(live[len(live)/2]); err != nil {
		t.Fatal(err)
	}
	_ = h.database.Sync()
	res, err := (RangeScanVerifier{}).Verify(testContext(h))
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Fatal("expected range count/key mismatch")
	}
}
