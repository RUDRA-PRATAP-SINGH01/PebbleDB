package crash

// Builtin engine hook strings — must stay aligned with internal/db/crashpoint.go.
// This package does not import internal/db to avoid coupling ATF crash infra to
// engine internals beyond the PEBBLEDB_CRASH_AT contract.
const (
	EngineFlushAfterSSTClose     = "flush_after_sst_close"
	EngineFlushAfterManifest     = "flush_after_manifest"
	EngineFlushAfterWalState     = "flush_after_wal_state"
	EngineFlushAfterWalTruncate  = "flush_after_wal_truncate"
	EngineCompactAfterMergeClose = "compact_after_merge_close"
	EngineCompactAfterManifest   = "compact_after_manifest"
	EngineCompactAfterSSTables   = "compact_after_sstables_update"
	EngineCompactAfterDeleteOld  = "compact_after_delete_old"
)

// BuiltinPoints returns self-describing definitions for engine crash sites that
// already exist in PebbleDB. This does not add new engine crash sites.
func BuiltinPoints() []CrashPoint {
	return []CrashPoint{
		{
			ID: "flush.after_sst_close", Name: "Flush after SST close",
			Description: "Crash after flush SST is closed, before/around manifest commit window.",
			Category:    CategoryFlush, Phase: PhaseFlushSST, Severity: SeverityHigh,
			EngineHook: EngineFlushAfterSSTClose, Enabled: true, SupportsRecovery: true,
			RequiresFlush: true, RequiresManifest: true, RequiresWAL: true,
			Metadata: map[string]string{"engine_const": "CrashAfterSSTClose"},
		},
		{
			ID: "flush.after_manifest", Name: "Flush after manifest",
			Description: "Crash after manifest AppendNewFile during flush.",
			Category:    CategoryFlush, Phase: PhaseFlushManifest, Severity: SeverityCritical,
			EngineHook: EngineFlushAfterManifest, Enabled: true, SupportsRecovery: true,
			RequiresFlush: true, RequiresManifest: true, RequiresWAL: true,
			Metadata: map[string]string{"engine_const": "CrashAfterManifestNewFile", "atf_wired": "true"},
		},
		{
			ID: "flush.after_wal_state", Name: "Flush after WAL state",
			Description: "Crash after WAL flush-state durability during flush.",
			Category:    CategoryFlush, Phase: PhaseFlushWALState, Severity: SeverityCritical,
			EngineHook: EngineFlushAfterWalState, Enabled: true, SupportsRecovery: true,
			RequiresFlush: true, RequiresManifest: true, RequiresWAL: true,
			Metadata: map[string]string{"engine_const": "CrashAfterWalFlushState", "atf_wired": "true"},
		},
		{
			ID: "flush.after_wal_truncate", Name: "Flush after WAL truncate",
			Description: "Crash after WAL truncate during flush.",
			Category:    CategoryFlush, Phase: PhaseFlushWALTruncate, Severity: SeverityHigh,
			EngineHook: EngineFlushAfterWalTruncate, Enabled: true, SupportsRecovery: true,
			RequiresFlush: true, RequiresManifest: true, RequiresWAL: true,
			Metadata: map[string]string{"engine_const": "CrashAfterWalTruncate"},
		},
		{
			ID: "compact.after_merge_close", Name: "Compact after merge close",
			Description: "Crash after compaction merge output SST is closed.",
			Category:    CategoryCompact, Phase: PhaseCompactMerge, Severity: SeverityHigh,
			EngineHook: EngineCompactAfterMergeClose, Enabled: true, SupportsRecovery: true,
			RequiresFlush: false, RequiresManifest: true, RequiresWAL: false,
			Metadata: map[string]string{"engine_const": "CrashAfterMergeClose"},
		},
		{
			ID: "compact.after_manifest", Name: "Compact after manifest",
			Description: "Crash after compaction manifest SetFileSet.",
			Category:    CategoryCompact, Phase: PhaseCompactManifest, Severity: SeverityCritical,
			EngineHook: EngineCompactAfterManifest, Enabled: true, SupportsRecovery: true,
			RequiresFlush: false, RequiresManifest: true, RequiresWAL: false,
			Metadata: map[string]string{"engine_const": "CrashAfterManifestSetFileSet"},
		},
		{
			ID: "compact.after_sstables_update", Name: "Compact after sstables update",
			Description: "Crash after in-memory SST set update during compaction.",
			Category:    CategoryCompact, Phase: PhaseCompactMemory, Severity: SeverityHigh,
			EngineHook: EngineCompactAfterSSTables, Enabled: true, SupportsRecovery: true,
			RequiresFlush: false, RequiresManifest: true, RequiresWAL: false,
			Metadata: map[string]string{"engine_const": "CrashAfterSSTablesUpdate"},
		},
		{
			ID: "compact.after_delete_old", Name: "Compact after delete old SSTs",
			Description: "Crash after deleting old SSTs during compaction.",
			Category:    CategoryCompact, Phase: PhaseCompactDelete, Severity: SeverityMedium,
			EngineHook: EngineCompactAfterDeleteOld, Enabled: true, SupportsRecovery: true,
			RequiresFlush: false, RequiresManifest: true, RequiresWAL: false,
			Metadata: map[string]string{"engine_const": "CrashAfterDeleteOldSSTs"},
		},
	}
}

// RegisterBuiltins registers all known engine crash points with EngineBridgeHooks.
func RegisterBuiltins(r *Registry) error {
	if r == nil {
		return newError(ErrInvalidConfig, "nil registry", nil)
	}
	for _, p := range BuiltinPoints() {
		if err := r.Register(p, NewEngineBridgeHook(p)); err != nil {
			return err
		}
	}
	return r.ValidateDependencies()
}

// NewBuiltinRegistry returns a registry preloaded with engine crash points.
func NewBuiltinRegistry() (*Registry, error) {
	r := NewRegistry()
	if err := RegisterBuiltins(r); err != nil {
		return nil, err
	}
	return r, nil
}
