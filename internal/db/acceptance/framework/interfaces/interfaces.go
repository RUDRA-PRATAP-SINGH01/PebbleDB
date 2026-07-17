// Package interfaces defines the core behavioral contracts of the PebbleDB
// Acceptance Testing Framework (ATF). These interfaces decouple the scenario execution,
// state generation, resource management, and verification subsystems, ensuring
// compile-time safety and extensibility.
//
// Dependency Rules:
// - This is a leaf package in the framework. It must not import any other internal framework packages.
// - All other framework packages may import this package to reference target interface types.
package interfaces

import (
	"context"
	"time"
)

// Scenario represents a single, self-contained acceptance test definition.
type Scenario interface {
	// ID returns the unique system identifier for this scenario (e.g., "EXS-010").
	ID() string

	// Name returns the descriptive, human-readable name of the scenario.
	Name() string

	// Version returns the semantic version of the scenario specification (e.g., "1.0.0").
	Version() string

	// Priority returns the campaign execution priority (e.g., P1, P2, P3).
	Priority() int

	// Requirements returns the list of mapped system requirement IDs (e.g., "DB-REC-005").
	Requirements() []string

	// Contracts returns the list of verified architectural contract IDs (e.g., "C-DUR-01").
	Contracts() []string

	// Capabilities returns the capability requirements of this scenario (e.g., "requires_wal").
	Capabilities() []string

	// Options returns the database configuration options required for the pre-crash write run.
	Options() map[string]interface{}

	// CrashPoint returns the target crash point identifier (e.g., "flush_after_manifest").
	CrashPoint() string

	// VerificationDAG returns the dependency graph of verifiers that must run.
	// Key is the verifier name, value is the slice of upstream verifiers it depends on.
	VerificationDAG() map[string][]string
}

// Dataset represents a generator of deterministic, reproducible key-value write sequences.
type Dataset interface {
	// Generate writes the dataset keys and values to the logical database writer.
	// Returns an expected state representation containing the ground-truth snapshot.
	Generate(ctx context.Context, writer interface{}) (interface{}, error)

	// KeyCount returns the total number of unique keys written by this dataset generator.
	KeyCount() int
}

// Verifier represents a pluggable post-recovery invariant checker.
type Verifier interface {
	// Name returns the unique registration name of this verifier (e.g., "get_verifier").
	Name() string

	// Verify executes validation assertions on the recovered database directory.
	// Accepts the current test context, the path to the recovered directory, and the expected state.
	Verify(ctx context.Context, dbDir string, expectedState interface{}) (interface{}, error)
}

// Inspector represents a low-level physical inspector operating on raw database files (e.g., Manifest, WAL).
type Inspector interface {
	// Name returns the identifier of this inspector (e.g., "manifest_inspector").
	Name() string

	// Inspect parses and validates the physical invariants of target files.
	Inspect(ctx context.Context, dbDir string) (interface{}, error)
}

// EvidenceCollector represents the compiler of diagnostic execution logs and filesystem snapshots.
type EvidenceCollector interface {
	// Collect packages raw logs, directory hashes, and snapshots into a zip bundle.
	Collect(ctx context.Context, session interface{}) (interface{}, error)
}

// EventSubscriber represents an observer that consumes framework lifecycle events.
type EventSubscriber interface {
	// Name returns the subscriber's registration name (e.g., "telemetry_engine").
	Name() string

	// OnEvent receives and processes a framework lifecycle event.
	OnEvent(ctx context.Context, event interface{}) error
}

// ScenarioRegistry manages scenario discoveries, lookups, and capability cross-checks.
type ScenarioRegistry interface {
	// Register registers a new scenario configuration.
	Register(scenario Scenario) error

	// Lookup retrieves a scenario by its unique ID.
	Lookup(id string) (Scenario, error)

	// List returns all registered scenarios.
	List() []Scenario

	// Filter returns scenarios matching the given criteria (priority, capabilities, tags).
	Filter(filter interface{}) []Scenario
}

// ScenarioRunner orchestrates the execution pipeline for a single scenario.
type ScenarioRunner interface {
	// Run executes the complete target scenario lifecycle (environment creation, write, crash, recovery, verify).
	Run(ctx context.Context, scenario Scenario, session interface{}) (interface{}, error)
}

// Scheduler coordinates scenario execution campaigns inside a concurrent worker pool.
type Scheduler interface {
	// Submit queues a scenario for campaign execution.
	Submit(scenario Scenario) error

	// Start launches background workers to process the queue.
	Start(ctx context.Context) error

	// Stop gracefully terminates queue processing, waiting for active workers to complete.
	Stop() error
}

// ResourceManager manages CPU cores, memory limits, and unique temporary directories during runs.
type ResourceManager interface {
	// Reserve allocates system resources matching the request. Blocks until resources are available.
	Reserve(ctx context.Context, req interface{}) (interface{}, error)

	// Release returns allocated resources back to the pool.
	Release(alloc interface{}) error

	// AllocateTempDir creates an isolated temporary directory namespace.
	AllocateTempDir(prefix string) (string, error)

	// CleanTempDir deletes the target temp directory.
	CleanTempDir(path string) error
}

// ConfigurationProvider loads, merges, and validates configuration settings from all sources.
type ConfigurationProvider interface {
	// Load parses parameters from defaults, scenario configs, environment variables, and CLI overrides.
	Load() (interface{}, error)

	// Validate checks that all required options are set and within acceptable boundaries.
	Validate(config interface{}) error
}

// Session represents a stateful context tracking a specific campaign, scenario, or execution step.
type Session interface {
	// ID returns the unique session UUID.
	ID() string

	// State returns the current session state enum value.
	State() int

	// Transition updates the session state, validating that the transition is allowed.
	Transition(newState int) error

	// CreatedAt returns the session initialization timestamp.
	CreatedAt() time.Time
}

// TelemetryEngine accumulates execution time, resource usage, and campaign validation metrics.
type TelemetryEngine interface {
	// RecordDuration logs elapsed time for a specific execution pipeline stage.
	RecordDuration(scenarioID string, stage string, elapsed time.Duration)

	// RecordMetric writes a numeric value for a specific metrics counter or gauge.
	RecordMetric(scenarioID string, name string, value float64)

	// Dump returns a structured metrics report.
	Dump() interface{}
}
