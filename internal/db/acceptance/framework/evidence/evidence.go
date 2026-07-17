// Package evidence packages the artifacts of a failed ATF scenario (the recovered
// database directory plus a structured report) into a single portable bundle so
// failures can be triaged offline.
package evidence

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// reportFileName is the JSON report embedded alongside the directory snapshot.
const reportFileName = "atf_report.json"

// Collector writes evidence bundles under a base directory.
type Collector struct {
	baseDir string
}

// NewCollector constructs a Collector rooted at baseDir. The directory is created
// lazily on first use.
func NewCollector(baseDir string) *Collector {
	return &Collector{baseDir: baseDir}
}

// BaseDir returns the collector root.
func (c *Collector) BaseDir() string { return c.baseDir }

// Package snapshots srcDir and marshals report into a single zip archive named
// <scenarioID>_<executionID>_<timestamp>.zip and returns its path. srcDir may be
// absent (already cleaned); in that case only the report is written.
func (c *Collector) Package(scenarioID, executionID, srcDir string, report any) (string, error) {
	if c == nil || c.baseDir == "" {
		return "", fmt.Errorf("evidence: nil or unconfigured collector")
	}
	if err := os.MkdirAll(c.baseDir, 0o755); err != nil {
		return "", fmt.Errorf("evidence: create base dir: %w", err)
	}

	name := fmt.Sprintf("%s_%s_%s.zip",
		sanitize(scenarioID), sanitize(executionID), time.Now().Format("20060102_150405"))
	zipPath := filepath.Join(c.baseDir, name)

	f, err := os.Create(zipPath)
	if err != nil {
		return "", fmt.Errorf("evidence: create archive: %w", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	// Embed the report first so it is always present even if the dir is gone.
	if report != nil {
		w, err := zw.Create(reportFileName)
		if err != nil {
			zw.Close()
			return "", err
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			zw.Close()
			return "", err
		}
	}

	if srcDir != "" {
		if info, err := os.Stat(srcDir); err == nil && info.IsDir() {
			if err := addDir(zw, srcDir); err != nil {
				zw.Close()
				return "", err
			}
		}
	}

	if err := zw.Close(); err != nil {
		return "", err
	}
	return zipPath, nil
}

func addDir(zw *zip.Writer, srcDir string) error {
	root := filepath.Clean(srcDir)
	base := filepath.Base(root)
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		arcName := filepath.ToSlash(filepath.Join(base, rel))
		if info.IsDir() {
			if rel == "." {
				return nil
			}
			_, err := zw.Create(arcName + "/")
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		w, err := zw.Create(arcName)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(w, src)
		return err
	})
}

func sanitize(s string) string {
	if s == "" {
		return "unknown"
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, s)
}
