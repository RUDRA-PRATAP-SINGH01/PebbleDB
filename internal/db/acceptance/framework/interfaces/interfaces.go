// Package interfaces defines ATF behavioral contracts with typed parameters.
package interfaces

import (
	"context"
	"time"
)

// Scenario is a declarative acceptance test definition.
type Scenario interface {
	ID() string
	Name() string
	Version() string
	Priority() int
	Requirements() []string
	Contracts() []string
	Capabilities() []string
	Options() map[string]string
	CrashPoint() string
	VerificationDAG() map[string][]string
}

// LogicalWriter decouples dataset generators from *db.DB.
type LogicalWriter interface {
	Put(key, value []byte) error
	Delete(key []byte) error
	// Flush forces memtable data toward durable SST storage (not merely WAL batch apply).
	Flush() error
	Sync() error
	Close() error
}

// Dataset generates a deterministic write workload.
type Dataset interface {
	Generate(ctx context.Context, writer LogicalWriter) (expected any, err error)
	KeyCount() int
}

// EventSubscriber observes framework lifecycle events.
type EventSubscriber interface {
	Name() string
	OnEvent(ctx context.Context, event any) error
}

// ScenarioRegistry stores scenario definitions.
type ScenarioRegistry interface {
	Register(scenario Scenario) error
	Lookup(id string) (Scenario, error)
	List() []Scenario
}

// TelemetryEngine records timings and counters.
type TelemetryEngine interface {
	RecordDuration(scenarioID string, stage string, elapsed time.Duration)
	RecordMetric(scenarioID string, name string, value float64)
	Dump() any
}
