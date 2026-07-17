package crash

import "testing"

func TestConfigValidate(t *testing.T) {
	if err := (Config{Enabled: false}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Config{
		Enabled: true, CrashPointID: "x", Policy: Policy{Kind: PolicyAlways},
	}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Config{
		Enabled: true, Policy: Policy{Kind: PolicyAlways},
	}).Validate(); err == nil {
		t.Fatal("expected missing point id")
	}
	if err := (Config{
		Enabled: true, ValidationOnly: true, DryRun: true, Policy: Policy{Kind: PolicyNever},
	}).Validate(); err == nil {
		t.Fatal("expected mutual exclusion")
	}
	if err := (Config{
		Enabled: true, Policy: Policy{Kind: PolicyScenarioControlled},
	}).Validate(); err != nil {
		t.Fatal(err)
	}
}
