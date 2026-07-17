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
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

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

	// 1. Resolve Scenario Dependency DAG
	sorted, err := s.resolveDependencies()
	if err != nil {
		s.running = false
		return err
	}

	s.mu.Lock()
	s.pendingQueue = sorted
	s.mu.Unlock()

	// 2. Launch execution pool slots (execution logic omitted for Milestone 1)
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
