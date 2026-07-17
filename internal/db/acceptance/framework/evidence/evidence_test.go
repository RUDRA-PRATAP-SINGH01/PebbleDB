package evidence

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageIncludesReportAndDir(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "run")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewCollector(filepath.Join(base, "evidence"))
	report := map[string]any{"scenario_id": "EXS-010", "passed": false}
	zipPath, err := c.Package("EXS-010", "exec-1", src, report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(zipPath, ".zip") {
		t.Fatalf("expected .zip path, got %s", zipPath)
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	found := map[string]bool{}
	for _, f := range zr.File {
		found[f.Name] = true
	}
	if !found[reportFileName] {
		t.Fatalf("report %q missing from archive: %v", reportFileName, found)
	}
	if !found["run/a.txt"] || !found["run/sub/b.txt"] {
		t.Fatalf("directory contents missing from archive: %v", found)
	}
}

func TestPackageMissingDirStillWritesReport(t *testing.T) {
	base := t.TempDir()
	c := NewCollector(filepath.Join(base, "evidence"))
	zipPath, err := c.Package("EXS-011", "exec-2", filepath.Join(base, "gone"), map[string]any{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 1 || zr.File[0].Name != reportFileName {
		t.Fatalf("expected only report file, got %d entries", len(zr.File))
	}
}

func TestPackageUnconfiguredCollector(t *testing.T) {
	var c *Collector
	if _, err := c.Package("x", "y", "", nil); err == nil {
		t.Fatal("expected error for nil collector")
	}
	c2 := NewCollector("")
	if _, err := c2.Package("x", "y", "", nil); err == nil {
		t.Fatal("expected error for empty base dir")
	}
}
