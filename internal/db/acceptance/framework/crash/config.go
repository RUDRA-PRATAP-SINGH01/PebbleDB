package crash

import "fmt"

// Config controls CrashManager behavior for one execution.
type Config struct {
	// CrashPointID is the registry ID (or EngineHook alias) to evaluate.
	CrashPointID string
	// Policy selects crash election behavior.
	Policy Policy
	// RandomSeed overrides Policy.Seed when non-zero for random policies.
	RandomSeed int64
	// Enabled master switch; when false, evaluations always skip.
	Enabled bool
	// DryRun evaluates and publishes events but does not apply child env mutations.
	DryRun bool
	// ValidationOnly only validates configuration; ShouldCrash never triggers.
	ValidationOnly bool
}

// DefaultConfig returns a scenario-controlled configuration.
// CrashPointID may be supplied later via Configure or taken from CrashContext.
func DefaultConfig() Config {
	return Config{
		Policy:  Policy{Kind: PolicyScenarioControlled},
		Enabled: true,
	}
}

// Validate checks Config invariants before execution.
func (c Config) Validate() error {
	if c.ValidationOnly && c.DryRun {
		return newError(ErrInvalidConfig, "ValidationOnly and DryRun are mutually exclusive", nil)
	}
	if !c.Enabled && !c.ValidationOnly {
		return nil
	}
	policy := c.Policy
	if c.RandomSeed != 0 {
		policy.Seed = c.RandomSeed
	}
	if err := policy.Validate(); err != nil {
		return err
	}
	// Scenario-controlled and Never may omit CrashPointID until evaluation.
	if c.CrashPointID == "" && c.Enabled &&
		c.Policy.Kind != PolicyNever &&
		c.Policy.Kind != PolicyScenarioControlled {
		return newError(ErrInvalidConfig, "CrashPointID is required for policy "+string(c.Policy.Kind), nil)
	}
	return nil
}

// effectivePolicy returns policy with seed override applied.
func (c Config) effectivePolicy() Policy {
	p := c.Policy
	if c.RandomSeed != 0 {
		p.Seed = c.RandomSeed
	}
	return p
}

// String returns a compact diagnostic representation.
func (c Config) String() string {
	return fmt.Sprintf("point=%s policy=%s enabled=%v dry_run=%v validation_only=%v seed=%d",
		c.CrashPointID, c.Policy.Kind, c.Enabled, c.DryRun, c.ValidationOnly, c.RandomSeed)
}
