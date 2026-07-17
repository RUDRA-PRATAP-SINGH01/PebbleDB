package verifier

import (
	"testing"
)

func TestIteratorVerifierPass(t *testing.T) {
	h := openHarness(t, 21, 50, 8, 5)
	res, err := (IteratorVerifier{}).Verify(testContext(h))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatalf("failures: %+v", res.Failures)
	}
	if res.ScansPerformed < 2 {
		t.Fatalf("expected forward+reverse scans, got %d", res.ScansPerformed)
	}
}

func TestIteratorVerifierExtraKey(t *testing.T) {
	h := openHarness(t, 22, 15, 0, 0)
	if err := h.database.Put([]byte("key_99999999"), []byte("extra")); err != nil {
		t.Fatal(err)
	}
	_ = h.database.Sync()
	res, err := (IteratorVerifier{}).Verify(testContext(h))
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Fatal("expected unexpected key / count mismatch")
	}
}

func TestIteratorVerifierOrderSensitive(t *testing.T) {
	h := openHarness(t, 23, 30, 0, 7)
	res, err := (IteratorVerifier{}).Verify(testContext(h))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatalf("ordering should pass on clean DB: %+v", res.Failures)
	}
}
