// Package scheduler coordinates acceptance scenario execution order, enforcing priority gates
// and resolving topologically sorted dependency graphs.
//
// Dependency Rules:
// - Imports: interfaces, types, errors, logging, eventbus, resource.
package scheduler

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/errors"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/eventbus"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/interfaces"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/resource"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/session"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// ScenarioExecutor runs a single scenario end-to-end. *runner.ScenarioRunner
// satisfies this interface.
type ScenarioExecutor interface {
	Run(ctx context.Context, scenario interfaces.Scenario, tracker *session.SessionTracker) (types.ScenarioResult, error)
}

// ExecutorFactory returns a fresh executor for each scenario. A per-scenario
// executor is required for safe concurrent execution because a ScenarioRunner
// owns mutable crash-manager state that must not be shared across goroutines.
type ExecutorFactory func() ScenarioExecutor

// CampaignScheduler implements interfaces.Scheduler.
type CampaignScheduler struct {
	mu           sync.Mutex
	scenarios    map[string]interfaces.Scenario
	completed    map[string]types.Status
	pendingQueue []interfaces.Scenario
	workersCount int
	logger       *logging.Logger
	eventBus     *eventbus.EventBus
	resourceMgr  *resource.ResourceManager
	running      bool
	stopChan     chan struct{}
	wg           sync.WaitGroup
	newExecutor  ExecutorFactory
	maxRetries   int
}

// SetExecutorFactory installs the factory used to build a per-scenario executor.
func (s *CampaignScheduler) SetExecutorFactory(f ExecutorFactory) {
	s.mu.Lock()
	s.newExecutor = f
	s.mu.Unlock()
}

// SetMaxRetries sets how many additional attempts a failing scenario receives.
func (s *CampaignScheduler) SetMaxRetries(n int) {
	if n < 0 {
		n = 0
	}
	s.mu.Lock()
	s.maxRetries = n
	s.mu.Unlock()
}

// NewCampaignScheduler allocates a CampaignScheduler.
func NewCampaignScheduler(
	logger *logging.Logger,
	eb *eventbus.EventBus,
	rm *resource.ResourceManager,
	workersCount int,
) *CampaignScheduler {
	return &CampaignScheduler{
		scenarios:    make(map[string]interfaces.Scenario),
		completed:    make(map[string]types.Status),
		pendingQueue: make([]interfaces.Scenario, 0),
		workersCount: workersCount,
		logger:       logger,
		eventBus:     eb,
		resourceMgr:  rm,
		stopChan:     make(chan struct{}),
	}
}

// Submit registers a scenario to be executed during the campaign.
func (s *CampaignScheduler) Submit(scenario interfaces.Scenario) error {
	if scenario == nil {
		return errors.NewRegistrationError("cannot submit nil scenario", nil)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := scenario.ID()
	if _, exists := s.scenarios[id]; exists {
		return errors.NewRegistrationError(fmt.Sprintf("scenario %s already submitted", id), nil)
	}

	s.scenarios[id] = scenario
	s.pendingQueue = append(s.pendingQueue, scenario)

	s.logger.Debug("Scenario %s submitted to Scheduler", id)
	return nil
}

// Start initiates queue resolution and topological ordering.
func (s *CampaignScheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()

	s.logger.Info("CampaignScheduler starting campaign execution loop")

	sorted, err := s.resolveDependencies()
	if err != nil {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return err
	}

	s.mu.Lock()
	s.pendingQueue = sorted
	s.mu.Unlock()

	s.logger.Info("Topological sorting completed successfully. Ready to run %d scenarios.", len(sorted))
	return nil
}

// Stop halts queue processing.
func (s *CampaignScheduler) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	close(s.stopChan)
	s.mu.Unlock()

	s.wg.Wait()
	s.logger.Info("CampaignScheduler stopped execution loop")
	return nil
}

// resolveDependencies performs a topological sort on the submitted scenarios based on execution priorities.
// Milestone 1 requires ordering by:
// 1. Priority (P1 must complete before P2, P2 before P3).
// 2. ID alphabetical order for determinism.
func (s *CampaignScheduler) resolveDependencies() ([]interfaces.Scenario, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sorted := make([]interfaces.Scenario, len(s.pendingQueue))
	copy(sorted, s.pendingQueue)

	// Sort deterministically:
	// - Priority ascending (P1=1, P2=2, P3=3).
	// - Alphabetical by ID.
	sort.Slice(sorted, func(i, j int) bool {
		pi := sorted[i].Priority()
		pj := sorted[j].Priority()
		if pi != pj {
			return pi < pj
		}
		return sorted[i].ID() < sorted[j].ID()
	})

	return sorted, nil
}

// Execute runs the whole campaign: scenarios are ordered by priority, executed
// tier by tier with up to workersCount concurrent workers, retried up to
// maxRetries on failure, and aggregated into a CampaignResult. A priority gate is
// enforced: if any scenario in a tier fails, every scenario in lower-priority
// tiers is marked BLOCKED and skipped. Execution stops promptly on ctx cancel.
func (s *CampaignScheduler) Execute(ctx context.Context, meta types.Metadata) (types.CampaignResult, error) {
	s.mu.Lock()
	factory := s.newExecutor
	s.mu.Unlock()
	if factory == nil {
		return types.CampaignResult{}, errors.NewRegistrationError("scheduler executor factory not set", nil)
	}

	sorted, err := s.resolveDependencies()
	if err != nil {
		return types.CampaignResult{}, err
	}

	tracker := session.NewCampaignTracker(meta)
	_ = tracker.Transition(types.StateCampaignRunning)

	gateOpen := true
	for _, tier := range groupByPriority(sorted) {
		if !gateOpen || ctx.Err() != nil {
			for _, sc := range tier {
				tracker.AddScenarioResult(blockedResult(sc, blockedReason(ctx)))
			}
			continue
		}

		results := s.runTier(ctx, tier)
		allPassed := true
		for _, r := range results {
			tracker.AddScenarioResult(r)
			if r.StatusVal != types.StatusPass {
				allPassed = false
			}
		}
		if !allPassed {
			gateOpen = false
			s.logger.Warn("Priority gate closed after tier with failures; lower-priority scenarios will be blocked")
		}
	}

	_ = tracker.Transition(types.StateCampaignCompleted)
	return tracker.CompileResult(), nil
}

// runTier executes one priority tier with bounded concurrency.
func (s *CampaignScheduler) runTier(ctx context.Context, tier []interfaces.Scenario) []types.ScenarioResult {
	workers := s.workersCount
	if workers < 1 {
		workers = 1
	}
	sem := make(chan struct{}, workers)
	results := make([]types.ScenarioResult, len(tier))
	var wg sync.WaitGroup
	for i, sc := range tier {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, scenario interfaces.Scenario) {
			defer wg.Done()
			defer func() { <-sem }()
			results[idx] = s.runOne(ctx, scenario)
		}(i, sc)
	}
	wg.Wait()
	return results
}

// runOne executes a single scenario with retries using a fresh executor.
func (s *CampaignScheduler) runOne(ctx context.Context, scenario interfaces.Scenario) types.ScenarioResult {
	s.mu.Lock()
	factory := s.newExecutor
	attempts := s.maxRetries + 1
	s.mu.Unlock()
	if attempts < 1 {
		attempts = 1
	}

	var last types.ScenarioResult
	for attempt := 0; attempt < attempts; attempt++ {
		if ctx.Err() != nil {
			return blockedResult(scenario, "context canceled")
		}
		tracker := session.NewSessionTracker(types.StateScenarioRunning)
		res, err := factory().Run(ctx, scenario, tracker)
		res.Retries = attempt
		last = res
		if err == nil && res.StatusVal == types.StatusPass {
			return res
		}
		s.logger.Warn("Scenario %s attempt %d/%d not passing (status=%s err=%v)",
			scenario.ID(), attempt+1, attempts, res.StatusVal, err)
	}
	if last.ScenarioID == "" {
		last.ScenarioID = scenario.ID()
		last.StatusVal = types.StatusFail
	}
	return last
}

// groupByPriority splits an already priority-sorted slice into consecutive tiers.
func groupByPriority(sorted []interfaces.Scenario) [][]interfaces.Scenario {
	var tiers [][]interfaces.Scenario
	var cur []interfaces.Scenario
	prev := 0
	for i, sc := range sorted {
		if i == 0 {
			prev = sc.Priority()
		}
		if sc.Priority() != prev {
			tiers = append(tiers, cur)
			cur = nil
			prev = sc.Priority()
		}
		cur = append(cur, sc)
	}
	if len(cur) > 0 {
		tiers = append(tiers, cur)
	}
	return tiers
}

func blockedResult(scenario interfaces.Scenario, reason string) types.ScenarioResult {
	return types.ScenarioResult{
		ScenarioID:   scenario.ID(),
		StatusVal:    types.StatusBlocked,
		FailureStage: reason,
	}
}

func blockedReason(ctx context.Context) string {
	if ctx.Err() != nil {
		return "context canceled"
	}
	return "priority gate closed by prior failure"
}
