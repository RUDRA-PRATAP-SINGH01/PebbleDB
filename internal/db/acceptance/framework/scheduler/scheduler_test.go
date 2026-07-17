package scheduler

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/interfaces"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/logging"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/session"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// behavior controls what the fake executor returns for a scenario.
type behavior struct {
	// failUntil: fail the first N attempts, then pass. -1 = always fail.
	failUntil int
}

type fakeControl struct {
	mu        sync.Mutex
	behaviors map[string]*behavior
	attempts  map[string]int
	maxActive int32
	active    int32

	// barrier forces `target` goroutines to be simultaneously in-flight before
	// any of them proceeds, so concurrency is observed deterministically.
	target   int32
	arrived  int32
	gate     chan struct{}
	gateOnce sync.Once
}

func (fc *fakeControl) run(ctx context.Context, sc interfaces.Scenario, _ *session.SessionTracker) (types.ScenarioResult, error) {
	cur := atomic.AddInt32(&fc.active, 1)
	for {
		old := atomic.LoadInt32(&fc.maxActive)
		if cur <= old || atomic.CompareAndSwapInt32(&fc.maxActive, old, cur) {
			break
		}
	}
	defer atomic.AddInt32(&fc.active, -1)

	if t := atomic.LoadInt32(&fc.target); t > 1 {
		if atomic.AddInt32(&fc.arrived, 1) >= t {
			fc.gateOnce.Do(func() { close(fc.gate) })
		}
		select {
		case <-fc.gate:
		case <-time.After(2 * time.Second):
		}
	}

	fc.mu.Lock()
	fc.attempts[sc.ID()]++
	attempt := fc.attempts[sc.ID()]
	b := fc.behaviors[sc.ID()]
	fc.mu.Unlock()

	status := types.StatusPass
	if b != nil {
		if b.failUntil < 0 || attempt <= b.failUntil {
			status = types.StatusFail
		}
	}
	return types.ScenarioResult{ScenarioID: sc.ID(), StatusVal: status}, nil
}

type fakeExecutor struct{ fc *fakeControl }

func (f fakeExecutor) Run(ctx context.Context, sc interfaces.Scenario, tr *session.SessionTracker) (types.ScenarioResult, error) {
	return f.fc.run(ctx, sc, tr)
}

func newTestScheduler(t *testing.T, workers int, fc *fakeControl) *CampaignScheduler {
	t.Helper()
	logger := logging.NewLogger(os.Stderr, logging.LevelError)
	s := NewCampaignScheduler(logger, nil, nil, workers)
	s.SetExecutorFactory(func() ScenarioExecutor { return fakeExecutor{fc: fc} })
	return s
}

func scn(id string, priority types.Priority) types.ScenarioDefinition {
	return types.ScenarioDefinition{IDStr: id, NameStr: id, PriorityVal: priority}
}

func TestSchedulerExecuteAllPass(t *testing.T) {
	fc := &fakeControl{behaviors: map[string]*behavior{}, attempts: map[string]int{}}
	s := newTestScheduler(t, 2, fc)
	for _, sc := range []types.ScenarioDefinition{scn("B", types.P2), scn("A", types.P1), scn("C", types.P2)} {
		if err := s.Submit(sc); err != nil {
			t.Fatal(err)
		}
	}
	res, err := s.Execute(context.Background(), types.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatalf("expected campaign pass, got %+v", res.Summary)
	}
	if res.Summary.PassedCount != 3 || res.Summary.TotalScenarios != 3 {
		t.Fatalf("unexpected summary: %+v", res.Summary)
	}
}

func TestSchedulerPriorityGateBlocksLowerTier(t *testing.T) {
	fc := &fakeControl{
		behaviors: map[string]*behavior{"A": {failUntil: -1}},
		attempts:  map[string]int{},
	}
	s := newTestScheduler(t, 1, fc)
	// A (P1) always fails → B/C (P2) must be blocked.
	for _, sc := range []types.ScenarioDefinition{scn("A", types.P1), scn("B", types.P2), scn("C", types.P2)} {
		if err := s.Submit(sc); err != nil {
			t.Fatal(err)
		}
	}
	res, err := s.Execute(context.Background(), types.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Fatal("expected campaign failure")
	}
	if res.Summary.FailedCount != 1 {
		t.Fatalf("expected 1 failed, got %d", res.Summary.FailedCount)
	}
	if res.Summary.BlockedCount != 2 {
		t.Fatalf("expected 2 blocked (P2 tier), got %d", res.Summary.BlockedCount)
	}
	// Blocked scenarios must not have been executed.
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.attempts["B"] != 0 || fc.attempts["C"] != 0 {
		t.Fatalf("blocked scenarios should not run: %+v", fc.attempts)
	}
}

func TestSchedulerRetriesUntilPass(t *testing.T) {
	fc := &fakeControl{
		behaviors: map[string]*behavior{"A": {failUntil: 2}}, // fail attempts 1,2; pass on 3
		attempts:  map[string]int{},
	}
	s := newTestScheduler(t, 1, fc)
	if err := s.Submit(scn("A", types.P1)); err != nil {
		t.Fatal(err)
	}
	s.SetMaxRetries(3)
	res, err := s.Execute(context.Background(), types.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatalf("expected pass after retries, got %+v", res.Summary)
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.attempts["A"] != 3 {
		t.Fatalf("expected 3 attempts, got %d", fc.attempts["A"])
	}
}

func TestSchedulerConcurrencyWithinTier(t *testing.T) {
	fc := &fakeControl{
		behaviors: map[string]*behavior{},
		attempts:  map[string]int{},
		target:    4,
		gate:      make(chan struct{}),
	}
	s := newTestScheduler(t, 4, fc)
	for _, id := range []string{"A", "B", "C", "D", "E", "F"} {
		if err := s.Submit(scn(id, types.P1)); err != nil {
			t.Fatal(err)
		}
	}
	res, err := s.Execute(context.Background(), types.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed || res.Summary.PassedCount != 6 {
		t.Fatalf("expected 6 passed, got %+v", res.Summary)
	}
	if atomic.LoadInt32(&fc.maxActive) < 4 {
		t.Fatalf("expected 4 concurrent, max active=%d", fc.maxActive)
	}
}

func TestSchedulerNoExecutorFactory(t *testing.T) {
	logger := logging.NewLogger(os.Stderr, logging.LevelError)
	s := NewCampaignScheduler(logger, nil, nil, 1)
	if err := s.Submit(scn("A", types.P1)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Execute(context.Background(), types.Metadata{}); err == nil {
		t.Fatal("expected error when executor factory is unset")
	}
}
