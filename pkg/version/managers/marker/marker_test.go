package marker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/version/manager"
	"github.com/cloudposse/atmos/pkg/version/managers"
)

var testRefs = map[string]manager.VersionRef{
	"opentofu": {Version: "1.10.6"},
	"nginx":    {Version: "1.29.0", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Pin: manager.PinDigest},
	"cli":      {Version: "v2.5.0"},
}

func planFixture(t *testing.T, name, content string) (string, []managers.FileChange) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	var m Manager
	changes, err := m.Plan(context.Background(), &managers.Input{
		Dir:   dir,
		Paths: []string{name},
		Refs:  testRefs,
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	return path, changes
}

func TestMarkerTrailingCommentYAML(t *testing.T) {
	_, changes := planFixture(t, "config.yaml",
		"tools:\n  opentofu: 1.9.0 # atmos:version opentofu\n")
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if !strings.Contains(string(changes[0].New), "opentofu: 1.10.6 # atmos:version opentofu") {
		t.Fatalf("expected version rewrite, got:\n%s", changes[0].New)
	}
}

func TestMarkerStandaloneCommentDockerfile(t *testing.T) {
	_, changes := planFixture(t, "Dockerfile",
		"# atmos:version opentofu\nENV TOFU_VERSION=1.9.0\nRUN echo 2.0.0\n")
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	updated := string(changes[0].New)
	if !strings.Contains(updated, "ENV TOFU_VERSION=1.10.6") {
		t.Fatalf("expected next-line rewrite, got:\n%s", updated)
	}
	if !strings.Contains(updated, "RUN echo 2.0.0") {
		t.Fatalf("expected unrelated line untouched, got:\n%s", updated)
	}
}

func TestMarkerStandaloneHTMLComment(t *testing.T) {
	_, changes := planFixture(t, "Dockerfile",
		"<!-- atmos:version opentofu -->\nENV TOFU_VERSION=1.9.0\n")
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if !strings.Contains(string(changes[0].New), "ENV TOFU_VERSION=1.10.6") {
		t.Fatalf("expected next-line rewrite, got:\n%s", changes[0].New)
	}
}

func TestMarkerPinnedDigestReplacement(t *testing.T) {
	_, changes := planFixture(t, "deploy.yaml",
		"image: nginx@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb # atmos:version nginx\n")
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if !strings.Contains(string(changes[0].New), "image: nginx@"+testRefs["nginx"].Digest) {
		t.Fatalf("expected digest rewrite, got:\n%s", changes[0].New)
	}
}

func TestMarkerSlashSlashCommentWithMatchOverride(t *testing.T) {
	_, changes := planFixture(t, "versions.go",
		"const cliVersion = \"v2.0.0\" // atmos:version cli match=\"(v[0-9.]+)\"\n")
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if !strings.Contains(string(changes[0].New), "const cliVersion = \"v2.5.0\"") {
		t.Fatalf("expected match-override rewrite, got:\n%s", changes[0].New)
	}
}

func TestMarkerIdempotency(t *testing.T) {
	path, changes := planFixture(t, "config.yaml",
		"tools:\n  opentofu: 1.9.0 # atmos:version opentofu\n")
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if err := managers.Apply([]managers.PlannedChange{{Manager: Name, FileChange: changes[0]}}); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	var m Manager
	again, err := m.Plan(context.Background(), &managers.Input{
		Dir:   filepath.Dir(path),
		Paths: []string{filepath.Base(path)},
		Refs:  testRefs,
	})
	if err != nil {
		t.Fatalf("second Plan returned error: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("expected idempotent apply, got %d changes", len(again))
	}
}

func TestMarkerUnknownEntryIgnored(t *testing.T) {
	_, changes := planFixture(t, "config.yaml",
		"tool: 1.0.0 # atmos:version does-not-exist\n")
	if len(changes) != 0 {
		t.Fatalf("expected unknown entry to be ignored, got %d changes", len(changes))
	}
}
