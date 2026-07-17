package crash

import "testing"

func TestPolicyValidate(t *testing.T) {
	cases := []struct {
		p       Policy
		wantErr bool
	}{
		{Policy{Kind: PolicyAlways}, false},
		{Policy{Kind: PolicyNever}, false},
		{Policy{Kind: PolicyProbability, Probability: 0.5}, false},
		{Policy{Kind: PolicyProbability, Probability: 1.5}, true},
		{Policy{Kind: PolicyNthInvocation, N: 0}, true},
		{Policy{Kind: PolicyNthInvocation, N: 3}, false},
		{Policy{Kind: PolicyTimeBased}, true},
		{Policy{Kind: "nope"}, true},
	}
	for _, tc := range cases {
		err := tc.p.Validate()
		if tc.wantErr && err == nil {
			t.Fatalf("expected err for %+v", tc.p)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("unexpected err for %+v: %v", tc.p, err)
		}
	}
}

func TestPolicyNthInvocation(t *testing.T) {
	e := newPolicyEvaluator()
	p := Policy{Kind: PolicyNthInvocation, N: 3}
	var crashes int
	for i := 0; i < 6; i++ {
		r, err := e.evaluate("x", "hook", p, "")
		if err != nil {
			t.Fatal(err)
		}
		if r.crash {
			crashes++
		}
	}
	if crashes != 2 {
		t.Fatalf("crashes=%d want 2", crashes)
	}
}

func TestPolicyProbabilityDeterministic(t *testing.T) {
	e1 := newPolicyEvaluator()
	e2 := newPolicyEvaluator()
	p := Policy{Kind: PolicyProbability, Probability: 0.3, Seed: 42}
	var a, b []bool
	for i := 0; i < 20; i++ {
		r1, _ := e1.evaluate("p", "h", p, "")
		r2, _ := e2.evaluate("p", "h", p, "")
		a = append(a, r1.crash)
		b = append(b, r2.crash)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("non-deterministic at %d", i)
		}
	}
}

func TestPolicyScenarioControlled(t *testing.T) {
	e := newPolicyEvaluator()
	p := Policy{Kind: PolicyScenarioControlled}
	r, err := e.evaluate("flush.after_manifest", EngineFlushAfterManifest, p, EngineFlushAfterManifest)
	if err != nil || !r.crash {
		t.Fatalf("expected match by engine hook: %+v %v", r, err)
	}
	r, err = e.evaluate("flush.after_manifest", EngineFlushAfterManifest, p, "other")
	if err != nil || r.crash {
		t.Fatalf("expected miss: %+v %v", r, err)
	}
}

func TestPolicyNeverAndAlways(t *testing.T) {
	e := newPolicyEvaluator()
	r, _ := e.evaluate("x", "h", Policy{Kind: PolicyNever}, "")
	if r.crash {
		t.Fatal("never should not crash")
	}
	r, _ = e.evaluate("x", "h", Policy{Kind: PolicyAlways}, "")
	if !r.crash {
		t.Fatal("always should crash")
	}
}
