package verifier

import "testing"

func TestMetadataVerifierPass(t *testing.T) {
	h := openHarness(t, 51, 10, 0, 0)
	vctx := testContext(h)
	res, err := (MetadataVerifier{}).Verify(vctx)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatalf("failures: %+v", res.Failures)
	}
	if !vctx.Report().DatabaseMetadata.OpenedCleanly {
		t.Fatal("expected opened cleanly")
	}
}
