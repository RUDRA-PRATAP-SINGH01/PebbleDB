package verifier

// Verifier is an independently executable post-recovery check module.
type Verifier interface {
	// Name returns a stable identifier used in reports and events.
	Name() string
	// Verify inspects the recovered database against the oracle in ctx.
	Verify(ctx *VerificationContext) (*ModuleResult, error)
}
