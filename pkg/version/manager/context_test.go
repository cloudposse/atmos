package manager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

// writeLockFixture writes a lock file with one entry so VersionMap resolves.
func writeLockFixture(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()
	tempDir := t.TempDir()
	lock := "version: 1\ntracks:\n  default:\n    opentofu:\n      version: 1.10.6\n"
	if err := os.WriteFile(filepath.Join(tempDir, "versions.lock.yaml"), []byte(lock), 0o600); err != nil {
		t.Fatalf("writing lock fixture: %v", err)
	}
	return &schema.AtmosConfiguration{BasePath: tempDir}
}

func TestAddTemplateContextNeverFlipsEmptyContext(t *testing.T) {
	atmosConfig := writeLockFixture(t)

	// An empty context gates whole-file template processing in the stack
	// processor; injecting version into it would evaluate unrelated templates
	// too early. It must stay empty even when .version is referenced.
	result, err := AddTemplateContext(atmosConfig, `tag: "{{ .version.opentofu }}"`, map[string]any{}, "")
	if err != nil {
		t.Fatalf("AddTemplateContext returned error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty context to stay empty, got %#v", result)
	}

	result, err = AddTemplateContext(atmosConfig, `tag: "{{ .version.opentofu }}"`, nil, "")
	if err != nil {
		t.Fatalf("AddTemplateContext returned error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected nil context to stay empty, got %#v", result)
	}
}

func TestAddTemplateContextIgnoresVersionPrefixedIdentifiers(t *testing.T) {
	atmosConfig := writeLockFixture(t)

	// Regression: `{{ .vars.versioning_enabled }}` contains the substring
	// ".version" and must NOT trigger version-context injection.
	context := map[string]any{"locals": map[string]any{"a": 1}}
	result, err := AddTemplateContext(atmosConfig, `versioning: "{{ .vars.versioning_enabled }}"`, context, "")
	if err != nil {
		t.Fatalf("AddTemplateContext returned error: %v", err)
	}
	if _, exists := result["version"]; exists {
		t.Fatal("expected .versioning_enabled to not inject the version context")
	}
}

func TestAddTemplateContextAddsVersionToNonEmptyContext(t *testing.T) {
	atmosConfig := writeLockFixture(t)

	context := map[string]any{"locals": map[string]any{"a": 1}}
	result, err := AddTemplateContext(atmosConfig, `tag: "{{ .version.opentofu }}"`, context, "")
	if err != nil {
		t.Fatalf("AddTemplateContext returned error: %v", err)
	}
	versionMap, ok := result["version"].(map[string]VersionRef)
	if !ok {
		t.Fatalf("expected version map in context, got %#v", result["version"])
	}
	if versionMap["opentofu"].Version != "1.10.6" {
		t.Fatalf("expected locked version 1.10.6, got %q", versionMap["opentofu"].Version)
	}

	// An existing version key is left untouched.
	preset := map[string]any{"version": "keep", "locals": map[string]any{}}
	result, err = AddTemplateContext(atmosConfig, `tag: "{{ .version.opentofu }}"`, preset, "")
	if err != nil {
		t.Fatalf("AddTemplateContext returned error: %v", err)
	}
	if result["version"] != "keep" {
		t.Fatalf("expected existing version key preserved, got %#v", result["version"])
	}
}
