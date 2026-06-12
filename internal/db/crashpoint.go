package db

import "os"

// Crash points for integration tests (PEBBLEDB_CRASH_AT env var).
const (
	CrashAfterSSTClose           = "flush_after_sst_close"
	CrashAfterManifestNewFile    = "flush_after_manifest"
	CrashAfterWalFlushState      = "flush_after_wal_state"
	CrashAfterWalTruncate        = "flush_after_wal_truncate"
	CrashAfterMergeClose         = "compact_after_merge_close"
	CrashAfterManifestSetFileSet = "compact_after_manifest"
	CrashAfterSSTablesUpdate     = "compact_after_sstables_update"
	CrashAfterDeleteOldSSTs      = "compact_after_delete_old"
)

// maybeCrash exits the process when PEBBLEDB_CRASH_AT matches point.
func maybeCrash(point string) {
	if os.Getenv("PEBBLEDB_CRASH_AT") == point {
		os.Exit(2)
	}
}
