package tests

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDocsGenerateRemoteTemplate(t *testing.T) {
	if skipReason != "" {
		t.Skipf("%s", skipReason)
	}

	ensureAtmosRunner(t)

	templateServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/README.md.gotmpl" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`# {{ .name }}

{{ .description }}

Remote template marker: {{ .remote_marker }}
`))
	}))
	defer templateServer.Close()

	workdir := t.TempDir()
	writeTestFile(t, filepath.Join(workdir, "README.yaml"), `name: Remote Template Project
description: Generated from a deterministic HTTP template.
remote_marker: from-httptest
`)
	writeTestFile(t, filepath.Join(workdir, "atmos.yaml"), fmt.Sprintf(`stacks:
  base_path: "stacks"
  included_paths:
    - "orgs/**/*"

logs:
  file: "/dev/stderr"
  level: "Off"

docs:
  generate:
    readme:
      base-dir: .
      input:
        - "./README.yaml"
      template: "%s/README.md.gotmpl"
      output: "./README.md"
`, templateServer.URL))

	t.Chdir(workdir)
	t.Setenv("GO_TEST", "1")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", workdir)
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(workdir, ".cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(workdir, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(workdir, ".local", "share"))
	t.Setenv("ATMOS_XDG_CACHE_HOME", filepath.Join(workdir, ".cache"))
	t.Setenv("ATMOS_XDG_CONFIG_HOME", filepath.Join(workdir, ".config"))
	t.Setenv("ATMOS_XDG_DATA_HOME", filepath.Join(workdir, ".local", "share"))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := prepareAtmosCommand(t, ctx, "docs", "generate", "readme")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("atmos docs generate readme failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	readme, err := os.ReadFile(filepath.Join(workdir, "README.md"))
	if err != nil {
		t.Fatalf("failed to read generated README.md: %v", err)
	}

	got := string(readme)
	for _, expected := range []string{
		"# Remote Template Project",
		"Generated from a deterministic HTTP template.",
		"Remote template marker: from-httptest",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("generated README.md missing %q:\n%s", expected, got)
		}
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create test directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
