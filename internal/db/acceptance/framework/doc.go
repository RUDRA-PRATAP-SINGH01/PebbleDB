// Package framework is the PebbleDB Acceptance Testing Framework (ATF).
//
// ATF certifies crash recovery by:
//  1. spawning an isolated child that writes a deterministic dataset
//  2. persisting expected_state.json before any crash
//  3. crashing at PEBBLEDB_CRASH_AT (engine maybeCrash) via ForceMemtableFlush
//  4. reopening the same directory in the parent
//  5. verifying Get + Scan against the oracle (PASS only if all match)
//
// Dependency flow (downward only):
//
//	interfaces, types, errors, logging  (leaves)
//	        ↓
//	eventbus, telemetry, resource, session, config, registry, util, dataset, verify, crash
//	        ↓
//	verifier, scheduler, runner
package framework
