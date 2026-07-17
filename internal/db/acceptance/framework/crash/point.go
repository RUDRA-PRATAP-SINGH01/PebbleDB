package crash

// Category groups crash points by storage subsystem.
type Category string

const (
	// CategoryFlush covers memtable flush pipeline crash sites.
	CategoryFlush Category = "flush"
	// CategoryCompact covers compaction pipeline crash sites.
	CategoryCompact Category = "compact"
	// CategoryWAL covers WAL durability crash sites.
	CategoryWAL Category = "wal"
	// CategoryManifest covers manifest durability crash sites.
	CategoryManifest Category = "manifest"
	// CategoryGeneric covers unclassified or bridge-only points.
	CategoryGeneric Category = "generic"
)

// Phase identifies where in the engine lifecycle the crash fires.
type Phase string

const (
	// PhaseFlushSST is after SST write/close during flush.
	PhaseFlushSST Phase = "flush_sst"
	// PhaseFlushManifest is after manifest AppendNewFile during flush.
	PhaseFlushManifest Phase = "flush_manifest"
	// PhaseFlushWALState is after WAL flush-state durability during flush.
	PhaseFlushWALState Phase = "flush_wal_state"
	// PhaseFlushWALTruncate is after WAL truncate during flush.
	PhaseFlushWALTruncate Phase = "flush_wal_truncate"
	// PhaseCompactMerge is after compaction merge output close.
	PhaseCompactMerge Phase = "compact_merge"
	// PhaseCompactManifest is after compaction manifest SetFileSet.
	PhaseCompactManifest Phase = "compact_manifest"
	// PhaseCompactMemory is after in-memory SST set update.
	PhaseCompactMemory Phase = "compact_memory"
	// PhaseCompactDelete is after old SST deletion during compaction.
	PhaseCompactDelete Phase = "compact_delete"
)

// Severity classifies operational risk of exercising a crash point.
type Severity string

const (
	// SeverityCritical marks points that historically caused data loss when broken.
	SeverityCritical Severity = "critical"
	// SeverityHigh marks high-risk recovery windows.
	SeverityHigh Severity = "high"
	// SeverityMedium marks moderate recovery complexity.
	SeverityMedium Severity = "medium"
	// SeverityLow marks low-risk or diagnostic points.
	SeverityLow Severity = "low"
)

// CrashPoint is a self-describing crash injection site.
type CrashPoint struct {
	ID               string
	Name             string
	Description      string
	Category         Category
	Phase            Phase
	Severity         Severity
	EngineHook       string // value consumed by PEBBLEDB_CRASH_AT / db.maybeCrash
	Enabled          bool
	Experimental     bool
	SupportsRecovery bool
	RequiresFlush    bool
	RequiresManifest bool
	RequiresWAL      bool
	// Dependencies lists other CrashPoint IDs that must be registered.
	Dependencies []string
	// Metadata holds optional string attributes (no free-form objects).
	Metadata map[string]string
}

// Clone returns a deep copy suitable for immutable registry snapshots.
func (p CrashPoint) Clone() CrashPoint {
	out := p
	if p.Dependencies != nil {
		out.Dependencies = append([]string(nil), p.Dependencies...)
	}
	if p.Metadata != nil {
		out.Metadata = make(map[string]string, len(p.Metadata))
		for k, v := range p.Metadata {
			out.Metadata[k] = v
		}
	}
	return out
}

// Validate checks that required identity fields are present.
func (p CrashPoint) Validate() error {
	if p.ID == "" {
		return newError(ErrInvalidConfig, "crash point ID is required", nil)
	}
	if p.Name == "" {
		return newError(ErrInvalidConfig, "crash point name is required", nil)
	}
	if p.EngineHook == "" {
		return newError(ErrInvalidConfig, "crash point EngineHook is required", nil)
	}
	if p.Category == "" {
		return newError(ErrInvalidConfig, "crash point category is required", nil)
	}
	if p.Phase == "" {
		return newError(ErrInvalidConfig, "crash point phase is required", nil)
	}
	return nil
}

// Capability describes registry-level support for a crash point.
type Capability struct {
	ID               string
	EngineHook       string
	Category         Category
	Phase            Phase
	Enabled          bool
	Experimental     bool
	SupportsRecovery bool
	RequiresFlush    bool
	RequiresManifest bool
	RequiresWAL      bool
}

// CapabilityOf builds a Capability view from a CrashPoint.
func CapabilityOf(p CrashPoint) Capability {
	return Capability{
		ID:               p.ID,
		EngineHook:       p.EngineHook,
		Category:         p.Category,
		Phase:            p.Phase,
		Enabled:          p.Enabled,
		Experimental:     p.Experimental,
		SupportsRecovery: p.SupportsRecovery,
		RequiresFlush:    p.RequiresFlush,
		RequiresManifest: p.RequiresManifest,
		RequiresWAL:      p.RequiresWAL,
	}
}
