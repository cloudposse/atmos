package template

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	texttemplate "text/template"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/manager"
	"github.com/cloudposse/atmos/pkg/version/managers"
)

// fakeRender is a minimal Go template engine standing in for the Atmos one.
func fakeRender(_ *schema.AtmosConfiguration, name, content string, data map[string]any) (string, error) {
	parsed, err := texttemplate.New(name).Parse(content)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := parsed.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func TestTemplateManagerRendersSiblingAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "versions.json.tmpl")
	if err := os.WriteFile(source, []byte(`{"opentofu": "{{ .version.opentofu }}", "pinned": "{{ .version.nginx }}"}`), 0o600); err != nil {
		t.Fatalf("writing template: %v", err)
	}
	input := &managers.Input{
		Dir:    dir,
		Render: fakeRender,
		Refs: map[string]manager.VersionRef{
			"opentofu": {Version: "1.10.6"},
			"nginx":    {Version: "1.29.0", Digest: "sha256:abc", Pin: manager.PinDigest},
		},
	}
	var m Manager

	changes, err := m.Plan(context.Background(), input)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Path != filepath.Join(dir, "versions.json") {
		t.Fatalf("expected sibling output path, got %q", changes[0].Path)
	}
	rendered := string(changes[0].New)
	if !strings.Contains(rendered, `"opentofu": "1.10.6"`) {
		t.Fatalf("expected rendered version, got:\n%s", rendered)
	}
	// Pinned refs render their digest via VersionRef.String().
	if !strings.Contains(rendered, `"pinned": "sha256:abc"`) {
		t.Fatalf("expected pinned digest form, got:\n%s", rendered)
	}

	if err := managers.Apply([]managers.PlannedChange{{Manager: Name, FileChange: changes[0]}}); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	again, err := m.Plan(context.Background(), input)
	if err != nil {
		t.Fatalf("second Plan returned error: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("expected idempotent apply, got %d changes", len(again))
	}
}
