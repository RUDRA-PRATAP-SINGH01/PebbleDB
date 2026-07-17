package crash

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
)

// PolicyKind selects how CrashManager decides whether to inject a crash.
type PolicyKind string

const (
	// PolicyAlways always elects to crash when the point is enabled.
	PolicyAlways PolicyKind = "always"
	// PolicyNever never elects to crash.
	PolicyNever PolicyKind = "never"
	// PolicyProbability crashes with Probability in [0,1], using Seed for determinism.
	PolicyProbability PolicyKind = "probability"
	// PolicyNthInvocation crashes on every Nth evaluation (N >= 1).
	PolicyNthInvocation PolicyKind = "nth_invocation"
	// PolicyRandomSeed crashes with p=0.5 using an explicit seed (deterministic).
	PolicyRandomSeed PolicyKind = "random_seed"
	// PolicyTimeBased is reserved for future wall-clock policies.
	PolicyTimeBased PolicyKind = "time_based"
	// PolicyScenarioControlled crashes only when the scenario requests this point ID or EngineHook.
	PolicyScenarioControlled PolicyKind = "scenario_controlled"
)

// Policy configures crash election.
type Policy struct {
	Kind PolicyKind
	// Probability is used by PolicyProbability (inclusive range [0,1]).
	Probability float64
	// N is used by PolicyNthInvocation (crash when invocationCount % N == 0).
	N int
	// Seed drives deterministic pseudo-random policies.
	Seed int64
}

// Validate checks policy fields for the selected kind.
func (p Policy) Validate() error {
	switch p.Kind {
	case PolicyAlways, PolicyNever, PolicyScenarioControlled:
		return nil
	case PolicyProbability:
		if p.Probability < 0 || p.Probability > 1 || math.IsNaN(p.Probability) {
			return newError(ErrInvalidConfig, "probability must be in [0,1]", nil)
		}
		return nil
	case PolicyNthInvocation:
		if p.N < 1 {
			return newError(ErrInvalidConfig, "nth_invocation requires N >= 1", nil)
		}
		return nil
	case PolicyRandomSeed:
		return nil
	case PolicyTimeBased:
		return newError(ErrPolicyUnsupported, "time_based policy is not supported in this milestone", nil)
	case "":
		return newError(ErrInvalidConfig, "policy kind is required", nil)
	default:
		return newError(ErrInvalidConfig, fmt.Sprintf("unknown policy kind %q", p.Kind), nil)
	}
}

// policyEvaluator holds mutable per-manager counters for Nth/probability.
type policyEvaluator struct {
	mu          sync.Mutex
	invocations map[string]int64
	rngBySeed   map[int64]*rand.Rand
}

func newPolicyEvaluator() *policyEvaluator {
	return &policyEvaluator{
		invocations: make(map[string]int64),
		rngBySeed:   make(map[int64]*rand.Rand),
	}
}

type policyResult struct {
	crash  bool
	reason string
}

func (e *policyEvaluator) evaluate(pointID, engineHook string, policy Policy, scenarioCrash string) (policyResult, error) {
	if err := policy.Validate(); err != nil {
		return policyResult{}, err
	}
	switch policy.Kind {
	case PolicyAlways:
		return policyResult{crash: true, reason: "policy=always"}, nil
	case PolicyNever:
		return policyResult{crash: false, reason: "policy=never"}, nil
	case PolicyScenarioControlled:
		ok := scenarioCrash != "" && (scenarioCrash == pointID || scenarioCrash == engineHook)
		return policyResult{
			crash:  ok,
			reason: fmt.Sprintf("scenario_controlled: request=%q match=%v", scenarioCrash, ok),
		}, nil
	case PolicyNthInvocation:
		e.mu.Lock()
		e.invocations[pointID]++
		n := e.invocations[pointID]
		e.mu.Unlock()
		ok := n%int64(policy.N) == 0
		return policyResult{
			crash:  ok,
			reason: fmt.Sprintf("nth_invocation: n=%d count=%d crash=%v", policy.N, n, ok),
		}, nil
	case PolicyProbability:
		ok := e.roll(policy.Seed, policy.Probability)
		return policyResult{
			crash:  ok,
			reason: fmt.Sprintf("probability: p=%g seed=%d crash=%v", policy.Probability, policy.Seed, ok),
		}, nil
	case PolicyRandomSeed:
		ok := e.roll(policy.Seed, 0.5)
		return policyResult{
			crash:  ok,
			reason: fmt.Sprintf("random_seed: seed=%d crash=%v", policy.Seed, ok),
		}, nil
	case PolicyTimeBased:
		return policyResult{}, newError(ErrPolicyUnsupported, "time_based policy is not supported", nil)
	default:
		return policyResult{}, newError(ErrInvalidConfig, fmt.Sprintf("unknown policy %q", policy.Kind), nil)
	}
}

func (e *policyEvaluator) roll(seed int64, p float64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	rng, ok := e.rngBySeed[seed]
	if !ok {
		rng = rand.New(rand.NewSource(seed))
		e.rngBySeed[seed] = rng
	}
	return rng.Float64() < p
}

// InvocationCount returns how many times pointID has been evaluated under Nth policy.
func (e *policyEvaluator) InvocationCount(pointID string) int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.invocations[pointID]
}
