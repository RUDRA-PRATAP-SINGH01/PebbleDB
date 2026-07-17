package verifier

import (
	"testing"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

func TestGetVerifierPass(t *testing.T) {
	h := openHarness(t, 11, 40, 5, 5)
	res, err := (GetVerifier{}).Verify(testContext(h))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatalf("failures: %+v", res.Failures)
	}
	if res.KeysVerified == 0 {
		t.Fatal("expected keys verified")
	}
}

func TestGetVerifierMissingKey(t *testing.T) {
	h := openHarness(t, 12, 20, 0, 0)
	// Remove a live key from DB by deleting without updating oracle.
	live := h.expected.LiveKeys()
	if len(live) == 0 {
		t.Fatal("no live keys")
	}
	if err := h.database.Delete(live[0]); err != nil {
		t.Fatal(err)
	}
	if err := h.database.Sync(); err != nil {
		t.Fatal(err)
	}
	res, err := (GetVerifier{}).Verify(testContext(h))
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Fatal("expected failure")
	}
	found := false
	for _, f := range res.Failures {
		if f.Reason == "missing_key" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want missing_key, got %+v", res.Failures)
	}
}

func TestGetVerifierWrongValue(t *testing.T) {
	h := openHarness(t, 13, 20, 0, 0)
	live := h.expected.LiveKeys()
	if err := h.database.Put(live[0], []byte("wrong")); err != nil {
		t.Fatal(err)
	}
	_ = h.database.Sync()
	res, err := (GetVerifier{}).Verify(testContext(h))
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Fatal("expected value mismatch")
	}
	ok := false
	for _, f := range res.Failures {
		if f.Reason == "value_mismatch" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("want value_mismatch: %+v", res.Failures)
	}
}

func TestGetVerifierDeletedKeyPresent(t *testing.T) {
	h := openHarness(t, 14, 20, 0, 5)
	var tombstone []byte
	for _, k := range h.expected.Keys() {
		snap, _ := h.expected.Get(k)
		if snap.Tombstone {
			tombstone = k
			break
		}
	}
	if tombstone == nil {
		t.Fatal("no tombstone in oracle")
	}
	if err := h.database.Put(tombstone, []byte("ghost")); err != nil {
		t.Fatal(err)
	}
	_ = h.database.Sync()
	res, err := (GetVerifier{}).Verify(testContext(h))
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Fatal("expected deleted_key_present")
	}
	ok := false
	for _, f := range res.Failures {
		if f.Reason == "deleted_key_present" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("want deleted_key_present: %+v", res.Failures)
	}
}

func TestGetVerifierExtraKeyDoesNotFailGet(t *testing.T) {
	// Get verifier only checks oracle keys; extras are caught by iterator/scan.
	h := openHarness(t, 15, 10, 0, 0)
	if err := h.database.Put([]byte("zzz_extra"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	_ = h.database.Sync()
	res, err := (GetVerifier{}).Verify(testContext(h))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatalf("get should pass: %+v", res.Failures)
	}
	_ = types.StatusPass
}
