package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestSaveLockWritesTwoSpaceIndent guards against SaveLock regressing to yaml.v3's bare
// yaml.Marshal default (4-space indent), which drifted from Atmos's repo-wide 2-space YAML
// standard (pkg/utils.DefaultYAMLIndent). SaveLock must go through pkg/utils's YAML helpers so
// generated versions.lock.yaml files match the indentation of every other Atmos-generated YAML
// file.
func TestSaveLockWritesTwoSpaceIndent(t *testing.T) {
	dir := t.TempDir()
	cfg := &schema.AtmosConfiguration{
		BasePath: dir,
		Version:  schema.Version{LockFile: "versions.lock.yaml"},
	}

	lock := &LockFile{
		Tracks: map[string]map[string]LockEntry{
			"prod": {
				"opentofu": {Version: "1.10.0", Digest: "sha256:abc"},
			},
		},
	}
	if err := SaveLock(cfg, lock); err != nil {
		t.Fatalf("SaveLock returned error: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "versions.lock.yaml"))
	if err != nil {
		t.Fatalf("reading lock file: %v", err)
	}
	content := string(raw)

	// "tracks:" -> "  prod:" -> "    opentofu:" -> "      version: ..." is 2-space-per-level.
	if !strings.Contains(content, "\n  prod:\n") {
		t.Fatalf("expected 2-space-indented track key, got:\n%s", content)
	}
	if !strings.Contains(content, "\n    opentofu:\n") {
		t.Fatalf("expected 4-space (2 levels x 2 spaces) indented entry key, got:\n%s", content)
	}
	if strings.Contains(content, "\n    prod:\n") || strings.Contains(content, "\n        opentofu:\n") {
		t.Fatalf("lock file uses 4-space-per-level indentation instead of the 2-space standard:\n%s", content)
	}
}
