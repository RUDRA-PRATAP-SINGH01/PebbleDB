// Package types defines the core domain models, configuration structures, and
// session envelopes for the PebbleDB Acceptance Testing Framework (ATF).
//
// Dependency Rules:
// - This package is a leaf node. It must not import other framework packages.
package types

import (
	"time"
)

// Priority defines the priority level of a scenario run.
type Priority int

const (
	// P1 represents foundation/critical durability scenarios.
	P1 Priority = 1
	// P2 represents advanced/recovery validation scenarios.
	P2 Priority = 2
	// P3 represents stress/chaos scenarios.
	P3 Priority = 3
)

// State defines the execution lifecycle state of a session or process.
type State int

const (
	StateInit State = iota
	StateCampaignRunning
	StateScenarioRunning
	StateExecutionPrepared
	StateSubprocessWriting
	StateSubprocessCrashed
	StateSubprocessExited
	StateDirectorySnapshoted
	StateRecoveryAttempted
	StateVerificationRunning
	StateEvidenceCollected
	StateScenarioCompleted
	StateScenarioFailed
	StateCampaignCompleted
)

// String returns a human-readable representation of the State.
func (s State) String() string {
	switch s {
	case StateInit:
		return "INIT"
	case StateCampaignRunning:
		return "CAMPAIGN_RUNNING"
	case StateScenarioRunning:
		return "SCENARIO_RUNNING"
	case StateExecutionPrepared:
		return "EXECUTION_PREPARED"
	case StateSubprocessWriting:
		return "SUBPROCESS_WRITING"
	case StateSubprocessCrashed:
		return "SUBPROCESS_CRASHED"
	case StateSubprocessExited:
		return "SUBPROCESS_EXITED"
	case StateDirectorySnapshoted:
		return "DIRECTORY_SNAPSHOTTED"
	case StateRecoveryAttempted:
		return "RECOVERY_ATTEMPTED"
	case StateVerificationRunning:
		return "VERIFICATION_RUNNING"
	case StateEvidenceCollected:
		return "EVIDENCE_COLLECTED"
	case StateScenarioCompleted:
		return "SCENARIO_COMPLETED"
	case StateScenarioFailed:
		return "SCENARIO_FAILED"
	case StateCampaignCompleted:
		return "CAMPAIGN_COMPLETED"
	default:
		return "UNKNOWN"
	}
}

// Status defines the success/failure resolution of a scenario or campaign.
type Status string

const (
	StatusPass         Status = "PASS"
	StatusFail         Status = "FAIL"
	StatusBlocked      Status = "BLOCKED"
	StatusInconclusive Status = "INCONCLUSIVE"
)

// ScenarioDefinition defines a declarative test specification.
type ScenarioDefinition struct {
	IDStr           string              `yaml:"id"`
	NameStr         string              `yaml:"name"`
	VersionStr      string              `yaml:"spec_version"`
	PriorityVal     Priority            `yaml:"priority"`
	RequirementsVal []string            `yaml:"requirements"`
	ContractsVal    []string            `yaml:"contracts"`
	CapabilitiesVal []string            `yaml:"capabilities"`
	OptionsMap      map[string]string   `yaml:"options"`
	CrashPointStr   string              `yaml:"crash_point"`
	VerifyDAGMap    map[string][]string `yaml:"verification_dag"`
}

// Implement the Scenario interface
func (s ScenarioDefinition) ID() string                           { return s.IDStr }
func (s ScenarioDefinition) Name() string                         { return s.NameStr }
func (s ScenarioDefinition) Version() string                      { return s.VersionStr }
func (s ScenarioDefinition) Priority() int                        { return int(s.PriorityVal) }
func (s ScenarioDefinition) Requirements() []string               { return s.RequirementsVal }
func (s ScenarioDefinition) Contracts() []string                  { return s.ContractsVal }
func (s ScenarioDefinition) Capabilities() []string               { return s.CapabilitiesVal }
func (s ScenarioDefinition) Options() map[string]string           { return s.OptionsMap }
func (s ScenarioDefinition) CrashPoint() string                   { return s.CrashPointStr }
func (s ScenarioDefinition) VerificationDAG() map[string][]string { return s.VerifyDAGMap }

// Metadata contains system info and binary fingerprints.
type Metadata struct {
	PebbleCommit string    `json:"pebble_commit"`
	GoVersion    string    `json:"go_version"`
	Platform     string    `json:"platform"`
	Timestamp    time.Time `json:"timestamp"`
}

// CampaignSession tracks the execution of a set of scenarios.
type CampaignSession struct {
	SessionID   string           `json:"session_id"`
	MetadataVal Metadata         `json:"metadata"`
	StateVal    State            `json:"state"`
	StartTime   time.Time        `json:"start_time"`
	EndTime     time.Time        `json:"end_time"`
	Scenarios   []ScenarioResult `json:"scenarios"`
}

// ScenarioResult records the outcome of a scenario execution campaign run.
type ScenarioResult struct {
	ScenarioID   string               `json:"scenario_id"`
	StatusVal    Status               `json:"status"`
	Retries      int                  `json:"retries"`
	Executions   []ExecutionResult    `json:"executions"`
	Verification *VerificationOutcome `json:"verification,omitempty"`
	TempDir      string               `json:"temp_dir,omitempty"`
	EvidencePath string               `json:"evidence_path,omitempty"`
	FailureStage string               `json:"failure_stage,omitempty"`
}

// VerificationOutcome is a package-leaf summary of a verification report,
// suitable for attaching to a ScenarioResult without importing the verifier
// package. It is produced by the runner from the verifier's VerificationReport.
type VerificationOutcome struct {
	Passed       bool              `json:"passed"`
	PassedChecks int               `json:"passed_checks"`
	FailedChecks int               `json:"failed_checks"`
	DurationMs   float64           `json:"duration_ms"`
	Aborted      bool              `json:"aborted"`
	AbortReason  string            `json:"abort_reason,omitempty"`
	Modules      []ModuleOutcome   `json:"modules,omitempty"`
	Failures     []VerifierFailure `json:"failures,omitempty"`
}

// ModuleOutcome summarizes a single verifier module's result.
type ModuleOutcome struct {
	Name         string  `json:"name"`
	Passed       bool    `json:"passed"`
	PassedChecks int     `json:"passed_checks"`
	FailedChecks int     `json:"failed_checks"`
	DurationMs   float64 `json:"duration_ms"`
}

// ExecutionSession represents a single isolated run of a scenario.
type ExecutionSession struct {
	SessionID  string    `json:"session_id"`
	ScenarioID string    `json:"scenario_id"`
	StateVal   State     `json:"state"`
	RunIndex   int       `json:"run_index"`
	TempDir    string    `json:"temp_dir"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
}

// ExecutionResult records the raw subprocess execution results.
type ExecutionResult struct {
	RunIndex      int     `json:"run_index"`
	ExitCode      int     `json:"exit_code"`
	Duration      float64 `json:"duration_ms"`
	StderrSummary string  `json:"stderr_summary"`
	StateAtExit   State   `json:"state_at_exit"`
}

// VerificationResult contains assertions and metrics generated post-recovery.
type VerificationResult struct {
	Passed   bool               `json:"passed"`
	Failures []VerifierFailure  `json:"failures"`
	Timings  map[string]float64 `json:"verifier_timings_ms"`
}

// VerifierFailure documents an invariant mismatch.
type VerifierFailure struct {
	Verifier string `json:"verifier"`
	Key      string `json:"key,omitempty"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Details  string `json:"details"`
}

// CampaignResult represents the final Campaign execution report.
type CampaignResult struct {
	SessionID string           `json:"session_id"`
	Passed    bool             `json:"passed"`
	Metadata  Metadata         `json:"metadata"`
	Summary   CampaignSummary  `json:"summary"`
	Details   []ScenarioResult `json:"details"`
}

// CampaignSummary aggregates statistics of the campaign.
type CampaignSummary struct {
	TotalScenarios int `json:"total_scenarios"`
	PassedCount    int `json:"passed_count"`
	FailedCount    int `json:"failed_count"`
	BlockedCount   int `json:"blocked_count"`
	DurationMs     int `json:"duration_ms"`
}

// EventType categorizes lifecycle events dispatched by the system.
type EventType int

const (
	EventSubprocessStarted EventType = iota
	EventSubprocessCrashed
	EventRecoveryStarted
	EventRecoveryFinished
	EventVerificationFinished
	EventEvidenceZipped
	EventVerificationStarted
	EventVerifierStarted
	EventVerifierPassed
	EventVerifierFailed
	EventVerificationAborted
	EventCrashPointRegistered
	EventCrashEvaluationStarted
	EventCrashEvaluationFinished
	EventCrashTriggered
	EventCrashSkipped
	EventCrashPolicyRejected
	EventCrashHookExecuted
)

// Event is the payload dispatched via the event bus.
type Event struct {
	Type      EventType   `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

// Configuration defines framework options.
type Configuration struct {
	BaseDir             string        `yaml:"base_dir"`
	MemtableSizeBytes   int64         `yaml:"memtable_size_bytes"`
	CompactionThreshold int           `yaml:"compaction_threshold"`
	SyncWrites          bool          `yaml:"sync_writes"`
	Parallelism         int           `yaml:"parallelism"`
	MaxRetries          int           `yaml:"max_retries"`
	Timeout             time.Duration `yaml:"timeout"`
}

// ResourceRequest specifies resource reservation parameters.
type ResourceRequest struct {
	CPUs           int   `json:"cpus"`
	MemoryMB       int64 `json:"memory_mb"`
	FileDescriptor int   `json:"file_descriptors"`
}

// ResourceAllocation tracks a granted resource reservation.
type ResourceAllocation struct {
	Request   ResourceRequest `json:"request"`
	GrantedAt time.Time       `json:"granted_at"`
	Released  bool            `json:"released"`
}

// ValueSnapshot represents the expected state of a key-value record.
type ValueSnapshot struct {
	Value     []byte `json:"value,omitempty"`
	Tombstone bool   `json:"tombstone"`
	Version   uint64 `json:"version"`
}
