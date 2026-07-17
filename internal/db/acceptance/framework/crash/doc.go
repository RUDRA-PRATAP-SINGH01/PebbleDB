// Package crash implements the ATF Crash Hook Framework: a data-driven layer
// that resolves, validates, and decides process-crash injection for acceptance
// scenarios without embedding crash-point knowledge in the execution engine.
//
// Architecture:
//
//	Scenario → Execution → CrashManager → CrashRegistry → CrashHook → Engine env bridge
//
// The storage engine still performs the actual process exit via PEBBLEDB_CRASH_AT
// / maybeCrash. This package owns configuration, policy evaluation, registry
// metadata, events, and telemetry. It does not parse manifests, WALs, or SSTs.
package crash
