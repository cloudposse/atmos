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
      dependencies:
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
		Ecosystem: "github/actions",
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
		"dependencies:",
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

	if _, err := SetEntryFields(atmosConfig, "prod", "opentofu", map[string]any{
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

	if _, err := SetEntryFields(atmosConfig, "prod", "missing", map[string]any{"desired": "1"}); !errors.Is(err, ErrEntryNotFound) {
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

// TestAddEntryPreservesConstraintOperators guards against JSON's default
// HTML-escaping of "<", ">", and "&" leaking through AddEntry's json.Marshal
// call into the raw YAML right-hand side written by pkg/yaml.SetRaw. Before
// the fix, `atmos version track add ... --desired ">=1.7.0"` silently wrote
// the literal escape sequence `>=1.7.0` into atmos.yaml instead of the
// intended constraint text, so this test asserts on the raw file bytes
// rather than just checking AddEntry returns no error.
func TestAddEntryPreservesConstraintOperators(t *testing.T) {
	cases := []struct {
		name    string
		desired string
	}{
		{name: "tilde_greater_constraint", desired: "~>1.7.0"},
		{name: "range_constraint", desired: ">=1.0.0,<2.0.0"},
		{name: "caret_constraint_regression_guard", desired: "^1.7.0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := crudSandbox(t)
			atmosConfig := &schema.AtmosConfiguration{}

			entry := &schema.VersionEntry{
				Ecosystem: "toolchain",
				Package:   "jqlang/jq",
				Desired:   tc.desired,
			}
			if _, err := AddEntry(atmosConfig, "prod", "jq", entry); err != nil {
				t.Fatalf("AddEntry returned error: %v", err)
			}

			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("reading config: %v", err)
			}
			text := string(content)

			// The literal constraint text must round-trip untouched.
			if !strings.Contains(text, tc.desired) {
				t.Errorf("expected config to contain literal desired value %q, got:\n%s", tc.desired, text)
			}

			// None of JSON's default HTML-escape unicode sequences may appear
			// in the written file; the fix must produce literal operators
			// instead of the six-character escape sequences below.
			for _, escaped := range []string{
				"\\" + "u003c", // less-than.
				"\\" + "u003e", // greater-than.
				"\\" + "u0026", // ampersand.
			} {
				if strings.Contains(text, escaped) {
					t.Errorf("expected config to not contain escaped sequence %q, got:\n%s", escaped, text)
				}
			}
		})
	}
}

// TestSetEntryFieldsPreservesAmpersandInExclude guards the setEntryField
// json.Marshal call (used for non-string fields such as slices) against the
// same HTML-escaping bug: an exclude pattern containing "&" must round-trip
// as a literal "&", not the escaped "&" sequence.
func TestSetEntryFieldsPreservesAmpersandInExclude(t *testing.T) {
	file := crudSandbox(t)
	atmosConfig := &schema.AtmosConfiguration{}

	exclude := []string{"foo&bar"}
	if _, err := SetEntryFields(atmosConfig, "prod", "opentofu", map[string]any{
		"exclude": exclude,
	}); err != nil {
		t.Fatalf("SetEntryFields returned error: %v", err)
	}

	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	text := string(content)

	if !strings.Contains(text, "foo&bar") {
		t.Errorf("expected config to contain literal %q, got:\n%s", "foo&bar", text)
	}
	if strings.Contains(text, "\\"+"u0026") {
		t.Errorf("expected config to not contain escaped ampersand, got:\n%s", text)
	}
}

func TestInferEcosystem(t *testing.T) {
	cases := map[string]string{
		"actions/checkout":                   "github/actions",
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
