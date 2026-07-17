package verifier

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/manifest"
)

// On-disk layout constants mirrored from internal/db. The structural verifiers
// intentionally re-declare them (rather than import unexported engine symbols)
// so the audits validate the persisted contract independently of the engine.
const (
	currentFileName       = "CURRENT"
	walFlushStateFileName = "wal.flush"
	walLogFileName        = "wal.log"
)

var sstFileRe = regexp.MustCompile(`^sst_(\d{8})\.sst$`)

// diskSSTables returns the set of top-level SSTable IDs present on disk,
// excluding the quarantine directory.
func diskSSTables(dir string) (map[uint64]int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make(map[uint64]int64)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := sstFileRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		id, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		out[id] = info.Size()
	}
	return out, nil
}

// manifestLiveIDs reads CURRENT, resolves the active MANIFEST file, and replays
// it into a sorted, de-duplicated live SSTable ID set. A missing CURRENT means
// the directory has no manifest yet (empty live set).
func manifestLiveIDs(dir string) ([]uint64, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, currentFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	name := strings.TrimSpace(string(data))
	if name == "" {
		return nil, false, fmt.Errorf("CURRENT is empty")
	}
	manifestPath := filepath.Join(dir, name)
	if _, err := os.Stat(manifestPath); err != nil {
		return nil, false, fmt.Errorf("CURRENT points to missing manifest %q: %w", name, err)
	}
	ids, err := manifest.ReplayFile(manifestPath)
	if err != nil {
		return nil, true, fmt.Errorf("replay manifest %q: %w", name, err)
	}
	return ids, true, nil
}

// DirectoryAudit asserts the recovered directory is internally consistent: the
// manifest live set and the on-disk SSTables agree, and no orphan SSTables are
// left in the live directory (recovery must have quarantined them).
type DirectoryAudit struct{}

// Name returns the module identifier.
func (DirectoryAudit) Name() string { return "directory_audit" }

// Verify cross-checks manifest live IDs against on-disk SSTable files.
func (DirectoryAudit) Verify(vctx *VerificationContext) (*ModuleResult, error) {
	start := time.Now()
	res := emptyModuleResult("directory_audit")
	dir := vctx.DatabasePath()

	onDisk, err := diskSSTables(dir)
	if err != nil {
		return finalizeModule(res, start), fmt.Errorf("directory_audit: read dir: %w", err)
	}
	liveIDs, hasManifest, err := manifestLiveIDs(dir)
	if err != nil {
		res.Failures = append(res.Failures, structFailure("directory_audit",
			"readable manifest", err.Error(), "manifest_unreadable",
			"Manifest CURRENT/MANIFEST could not be resolved after recovery"))
		return finalizeModule(res, start), nil
	}

	live := make(map[uint64]struct{}, len(liveIDs))
	for _, id := range liveIDs {
		live[id] = struct{}{}
	}

	// Every live SSTable must exist on disk.
	for _, id := range liveIDs {
		if _, ok := onDisk[id]; !ok {
			res.Failures = append(res.Failures, structFailure("directory_audit",
				fmt.Sprintf("sst_%08d.sst present", id), "missing",
				"missing_live_sst",
				"Manifest references a live SSTable that is not present on disk"))
		} else {
			res.PassedChecks++
		}
	}

	// No orphan SSTables must remain in the live directory.
	for id := range onDisk {
		if _, ok := live[id]; !ok {
			res.Failures = append(res.Failures, structFailure("directory_audit",
				fmt.Sprintf("sst_%08d.sst quarantined/absent", id), "present in live dir",
				"orphan_sst",
				"On-disk SSTable is not in the manifest live set and was not quarantined"))
		} else {
			res.PassedChecks++
		}
	}

	if !hasManifest && len(onDisk) > 0 {
		res.Failures = append(res.Failures, structFailure("directory_audit",
			"manifest present", "no CURRENT/manifest", "no_manifest",
			"SSTables exist on disk but there is no manifest to describe the live set"))
	} else {
		res.PassedChecks++
	}

	return finalizeModule(res, start), nil
}

// ManifestAudit asserts the manifest itself is well-formed after recovery:
// CURRENT resolves, the manifest replays cleanly, live IDs are unique and
// ascending, and each live SSTable file is non-empty.
type ManifestAudit struct{}

// Name returns the module identifier.
func (ManifestAudit) Name() string { return "manifest_audit" }

// Verify validates manifest integrity independently of the engine.
func (ManifestAudit) Verify(vctx *VerificationContext) (*ModuleResult, error) {
	start := time.Now()
	res := emptyModuleResult("manifest_audit")
	dir := vctx.DatabasePath()

	liveIDs, hasManifest, err := manifestLiveIDs(dir)
	if err != nil {
		res.Failures = append(res.Failures, structFailure("manifest_audit",
			"replayable manifest", err.Error(), "manifest_corrupt",
			"Manifest failed to replay after recovery"))
		return finalizeModule(res, start), nil
	}
	if !hasManifest {
		// A directory with no manifest is only valid when it has no SSTables;
		// DirectoryAudit covers the inconsistent case, so treat as pass here.
		res.PassedChecks++
		return finalizeModule(res, start), nil
	}

	sorted := sort.SliceIsSorted(liveIDs, func(i, j int) bool { return liveIDs[i] < liveIDs[j] })
	if !sorted {
		res.Failures = append(res.Failures, structFailure("manifest_audit",
			"ascending live IDs", fmt.Sprintf("%v", liveIDs), "unsorted_live_set",
			"Manifest live IDs are not in ascending order"))
	} else {
		res.PassedChecks++
	}

	seen := make(map[uint64]struct{}, len(liveIDs))
	for _, id := range liveIDs {
		if _, dup := seen[id]; dup {
			res.Failures = append(res.Failures, structFailure("manifest_audit",
				fmt.Sprintf("unique id %d", id), "duplicate", "duplicate_live_id",
				"Manifest live set contains a duplicate SSTable ID"))
			continue
		}
		seen[id] = struct{}{}
		info, err := os.Stat(filepath.Join(dir, fmt.Sprintf("sst_%08d.sst", id)))
		if err != nil {
			res.Failures = append(res.Failures, structFailure("manifest_audit",
				fmt.Sprintf("sst_%08d.sst readable", id), err.Error(), "live_sst_stat_failed",
				"A live SSTable could not be stat'd"))
			continue
		}
		if info.Size() == 0 {
			res.Failures = append(res.Failures, structFailure("manifest_audit",
				fmt.Sprintf("sst_%08d.sst non-empty", id), "0 bytes", "empty_live_sst",
				"A live SSTable file is empty"))
			continue
		}
		res.PassedChecks++
	}

	return finalizeModule(res, start), nil
}

// CheckpointAudit validates the WAL flush checkpoint (wal.flush) that bridges
// SST durability and WAL truncation. If present it must be exactly 16 bytes and
// reference a live SSTable; a corrupt checkpoint is a hard failure.
type CheckpointAudit struct{}

// Name returns the module identifier.
func (CheckpointAudit) Name() string { return "checkpoint_audit" }

// Verify checks wal.flush well-formedness and cross-references the manifest.
func (CheckpointAudit) Verify(vctx *VerificationContext) (*ModuleResult, error) {
	start := time.Now()
	res := emptyModuleResult("checkpoint_audit")
	dir := vctx.DatabasePath()

	data, err := os.ReadFile(filepath.Join(dir, walFlushStateFileName))
	if err != nil {
		if os.IsNotExist(err) {
			// No checkpoint pending — the common healthy state after recovery.
			res.PassedChecks++
			return finalizeModule(res, start), nil
		}
		return finalizeModule(res, start), fmt.Errorf("checkpoint_audit: read wal.flush: %w", err)
	}

	if len(data) != 16 {
		res.Failures = append(res.Failures, structFailure("checkpoint_audit",
			"16-byte checkpoint", fmt.Sprintf("%d bytes", len(data)), "corrupt_checkpoint",
			"wal.flush checkpoint is not exactly 16 bytes"))
		return finalizeModule(res, start), nil
	}
	freezeOffset := int64(binary.BigEndian.Uint64(data[0:8]))
	sstID := binary.BigEndian.Uint64(data[8:16])
	if freezeOffset < 0 {
		res.Failures = append(res.Failures, structFailure("checkpoint_audit",
			"non-negative freeze offset", fmt.Sprintf("%d", freezeOffset), "bad_freeze_offset",
			"wal.flush checkpoint has a negative freeze offset"))
	} else {
		res.PassedChecks++
	}

	liveIDs, hasManifest, err := manifestLiveIDs(dir)
	if err != nil || !hasManifest {
		// Manifest problems are reported by the manifest/directory audits.
		return finalizeModule(res, start), nil
	}
	inManifest := false
	for _, id := range liveIDs {
		if id == sstID {
			inManifest = true
			break
		}
	}
	if !inManifest {
		// Recovery safely falls back to a full WAL replay in this case, so this is
		// a diagnostic warning rather than a correctness failure.
		res.Failures = append(res.Failures, Failure{
			Verifier:       "checkpoint_audit",
			ExpectedValue:  "checkpoint SST in manifest",
			RecoveredValue: fmt.Sprintf("sst %d not live", sstID),
			Reason:         "checkpoint_sst_not_live",
			Severity:       SeverityWarning,
			RecoveryPhase:  PhaseVerify,
			Explanation:    "wal.flush references an SSTable absent from the live set (recovery falls back to full WAL replay)",
		})
	} else {
		res.PassedChecks++
	}

	// Best-effort presence note for the WAL (not a failure either way).
	if _, err := os.Stat(filepath.Join(dir, walLogFileName)); err == nil {
		res.PassedChecks++
	}

	return finalizeModule(res, start), nil
}

// structFailure builds an error-severity structural failure in the verify phase.
func structFailure(verifier, expected, recovered, reason, explanation string) Failure {
	return Failure{
		Verifier:       verifier,
		ExpectedValue:  expected,
		RecoveredValue: recovered,
		Reason:         reason,
		Severity:       SeverityError,
		RecoveryPhase:  PhaseVerify,
		Explanation:    explanation,
	}
}
