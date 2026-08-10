package githubactions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/version/manager"
	"github.com/cloudposse/atmos/pkg/version/managers"
)

const workflowFixture = `name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@0000000000000000000000000000000000000000 # v5.0.0
      - uses: aws-actions/configure-aws-credentials/subdir@v1 # keep this explanatory note
      - uses: unmanaged/action@v9
      - uses: ./local/action
      - name: reusable
        uses: acme/workflows/.github/workflows/ci.yml@v2
`

// fixtureInput builds an Input over a temp workflow file, returning the path.
func fixtureInput(t *testing.T) (*managers.Input, string) {
	t.Helper()
	dir := t.TempDir()
	workflowDir := filepath.Join(dir, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("creating workflow dir: %v", err)
	}
	path := filepath.Join(workflowDir, "ci.yaml")
	if err := os.WriteFile(path, []byte(workflowFixture), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	input := &managers.Input{
		Dir: dir,
		Entries: map[string]manager.EffectiveEntry{
			"checkout":  {Name: "checkout", Datasource: "github-tags", Package: "actions/checkout"},
			"setup-go":  {Name: "setup-go", Datasource: "github-tags", Package: "actions/setup-go"},
			"aws-creds": {Name: "aws-creds", Datasource: "github-tags", Package: "aws-actions/configure-aws-credentials"},
			"workflows": {Name: "workflows", Datasource: "github-tags", Package: "acme/workflows"},
		},
		Refs: map[string]manager.VersionRef{
			"checkout":  {Version: "v6.1.0", Digest: "1111111111111111111111111111111111111111", Pin: manager.PinDigest},
			"setup-go":  {Version: "v5.2.0", Digest: "2222222222222222222222222222222222222222", Pin: manager.PinDigest},
			"aws-creds": {Version: "v4.0.1"},
			"workflows": {Version: "v3"},
		},
	}
	return input, path
}

func TestGitHubActionsManagerRewritesManagedRefs(t *testing.T) {
	input, path := fixtureInput(t)
	var m Manager

	changes, err := m.Plan(context.Background(), input)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 changed file, got %d", len(changes))
	}
	updated := string(changes[0].New)

	expectations := []string{
		// Pinned: SHA plus trailing version comment.
		"uses: actions/checkout@1111111111111111111111111111111111111111 # v6.1.0",
		// Pinned with stale SHA/comment: fully rewritten.
		"uses: actions/setup-go@2222222222222222222222222222222222222222 # v5.2.0",
		// Unpinned subdirectory action: version ref, package matched on owner/repo.
		"uses: aws-actions/configure-aws-credentials/subdir@v4.0.1 # keep this explanatory note",
		// Reusable workflow path.
		"uses: acme/workflows/.github/workflows/ci.yml@v3",
		// Unmanaged and local refs untouched.
		"uses: unmanaged/action@v9",
		"uses: ./local/action",
	}
	for _, expected := range expectations {
		if !strings.Contains(updated, expected) {
			t.Errorf("expected rewritten workflow to contain %q\n---\n%s", expected, updated)
		}
	}

	// Round-trip idempotency: after applying, a second plan is empty.
	planned := []managers.PlannedChange{{Manager: Name, FileChange: changes[0]}}
	if err := managers.Apply(planned); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	again, err := m.Plan(context.Background(), input)
	if err != nil {
		t.Fatalf("second Plan returned error: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("expected idempotent apply, got %d changes", len(again))
	}

	// The applied file is byte-identical to the planned content.
	applied, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading applied file: %v", err)
	}
	if string(applied) != updated {
		t.Fatal("expected applied file to match planned content")
	}
}

func TestActionPackage(t *testing.T) {
	cases := map[string]string{
		"actions/checkout":                       "actions/checkout",
		"actions/checkout/subdir":                "actions/checkout",
		"acme/workflows/.github/workflows/x.yml": "acme/workflows",
		"single":                                 "single",
	}
	for input, expected := range cases {
		if got := actionPackage(input); got != expected {
			t.Errorf("actionPackage(%q) = %q, expected %q", input, got, expected)
		}
	}
}
