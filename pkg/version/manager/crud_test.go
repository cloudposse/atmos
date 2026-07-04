package manager

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

const crudConfigFixture = `# Project configuration — hand-written comment that must survive edits.
base_path: "."

version:
  track: prod
  tracks:
    prod:
      versions:
        # Keep opentofu on 1.10 until the provider matrix is validated.
        opentofu:
          ecosystem: toolchain
          package: opentofu
          desired: "~1.10"
`

// crudSandbox writes the fixture atmos.yaml into a temp working directory and
// chdirs there so ResolveEditableConfigFile finds it.
func crudSandbox(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	if err := os.WriteFile(file, []byte(crudConfigFixture), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	t.Chdir(dir)
	return file
}

func TestAddEntryPreservesCommentsAndFailsOnDuplicate(t *testing.T) {
	file := crudSandbox(t)
	atmosConfig := &schema.AtmosConfiguration{}

	entry := &schema.VersionEntry{
		Ecosystem: "github-actions",
		Package:   "actions/checkout",
		Desired:   "v6",
		Update:    schema.VersionUpdatePolicy{Pin: "sha"},
	}
	modified, err := AddEntry(atmosConfig, "prod", "checkout", entry)
	if err != nil {
		t.Fatalf("AddEntry returned error: %v", err)
	}
	if modified != file {
		t.Fatalf("expected edit in %s, got %s", file, modified)
	}
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	text := string(content)
	for _, expected := range []string{
		"# Project configuration — hand-written comment that must survive edits.",
		"# Keep opentofu on 1.10 until the provider matrix is validated.",
		"checkout:",
		"package: actions/checkout",
		"desired: v6",
		"pin: sha",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("expected config to contain %q after add:\n%s", expected, text)
		}
	}

	if _, err := AddEntry(atmosConfig, "prod", "checkout", entry); !errors.Is(err, ErrEntryExists) {
		t.Fatalf("expected ErrEntryExists, got %v", err)
	}
}

func TestSetEntryFieldsAndRemoveEntry(t *testing.T) {
	file := crudSandbox(t)
	atmosConfig := &schema.AtmosConfiguration{}

	if _, err := SetEntryFields(atmosConfig, "prod", "opentofu", map[string]string{
		"desired":    "~1.11",
		"update.pin": "none",
	}); err != nil {
		t.Fatalf("SetEntryFields returned error: %v", err)
	}
	content, _ := os.ReadFile(file)
	if !strings.Contains(string(content), `desired: "~1.11"`) && !strings.Contains(string(content), "desired: ~1.11") {
		t.Fatalf("expected updated desired, got:\n%s", content)
	}
	if !strings.Contains(string(content), "# Keep opentofu on 1.10") {
		t.Fatalf("expected entry comment preserved, got:\n%s", content)
	}

	if _, err := SetEntryFields(atmosConfig, "prod", "missing", map[string]string{"desired": "1"}); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("expected ErrEntryNotFound, got %v", err)
	}

	if _, err := RemoveEntry(atmosConfig, "prod", "opentofu"); err != nil {
		t.Fatalf("RemoveEntry returned error: %v", err)
	}
	content, _ = os.ReadFile(file)
	if strings.Contains(string(content), "package: opentofu") {
		t.Fatalf("expected entry removed, got:\n%s", content)
	}
	if _, err := RemoveEntry(atmosConfig, "prod", "opentofu"); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("expected ErrEntryNotFound after removal, got %v", err)
	}
}

func TestInferEcosystem(t *testing.T) {
	cases := map[string]string{
		"actions/checkout":                   "github-actions",
		"ghcr.io/acme/app":                   "oci",
		"library/nginx":                      "github",
		"cloudposse/atmos":                   "github",
		"opentofu":                           "toolchain",
		"registry-1.docker.io/library/nginx": "oci",
	}
	for pkg, expected := range cases {
		if got := InferEcosystem(pkg); got != expected {
			t.Errorf("InferEcosystem(%q) = %q, expected %q", pkg, got, expected)
		}
	}
}
