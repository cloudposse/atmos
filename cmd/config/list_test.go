package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	listpkg "github.com/cloudposse/atmos/pkg/list"
)

func TestBuildConfigPathRows(t *testing.T) {
	dir := t.TempDir()
	mainFile := filepath.Join(dir, "atmos.yaml")
	importFile := filepath.Join(dir, "atmos.d", "integrations.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(importFile), 0o755))
	require.NoError(t, os.WriteFile(mainFile, []byte(`
logs:
  level: info
components:
  terraform:
    base_path: components/terraform
`), 0o644))
	require.NoError(t, os.WriteFile(importFile, []byte(`
integrations:
  github:
    gitops:
      enabled: true
`), 0o644))

	rows, err := buildConfigPathRows([]string{mainFile, importFile}, dir)
	require.NoError(t, err)

	output, err := listpkg.RenderPathRows(rows, "paths", "")
	require.NoError(t, err)
	require.Equal(t, `atmos.d/integrations.yaml
  integrations
  integrations.github
  integrations.github.gitops
  integrations.github.gitops.enabled

atmos.yaml
  components
  components.terraform
  components.terraform.base_path
  logs
  logs.level
`, output)
}

func TestBuildConfigPathRowsQuotedKeys(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`
"foo.bar":
  "baz\"qux": true
`), 0o644))

	rows, err := buildConfigPathRows([]string{file}, dir)
	require.NoError(t, err)
	require.Contains(t, rows, listpkg.PathRow{File: "atmos.yaml", Path: `"foo.bar"`, Type: "object", Value: "{1 keys}"})
	require.Contains(t, rows, listpkg.PathRow{File: "atmos.yaml", Path: `"foo.bar"."baz\"qux"`, Type: "bool", Value: "true"})
}

func TestConfigListCommand_RunE(t *testing.T) {
	// configListCmd.RunE ends up calling data.Write, which panics unless the
	// I/O writer has been initialized (mirrors the pattern used across other
	// cmd/* packages, e.g. cmd/ai/skill/list_test.go, cmd/list/components_test.go).
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	dir := t.TempDir()
	file := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(file, []byte("logs:\n  level: info\n"), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
	require.NoError(t, os.Chdir(dir))

	require.NoError(t, configListCmd.RunE(configListCmd, nil))
	require.NoError(t, configListCmd.RunE(configListCmd, []string{"logs.*"}))
}

func TestRelativePathForDisplay(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		basePath string
		want     string
	}{
		{
			name:     "empty base path returns file unchanged",
			file:     filepath.Join("some", "dir", "atmos.yaml"),
			basePath: "",
			want:     filepath.ToSlash(filepath.Join("some", "dir", "atmos.yaml")),
		},
		{
			name:     "normal relative case",
			file:     filepath.Join(string(filepath.Separator), "a", "b", "c", "atmos.yaml"),
			basePath: filepath.Join(string(filepath.Separator), "a", "b"),
			want:     "c/atmos.yaml",
		},
		{
			name:     "rel is exactly ..",
			file:     filepath.Join(string(filepath.Separator), "a"),
			basePath: filepath.Join(string(filepath.Separator), "a", "b"),
			want:     filepath.ToSlash(filepath.Join(string(filepath.Separator), "a")),
		},
		{
			name:     "rel has ../ prefix",
			file:     filepath.Join(string(filepath.Separator), "a", "sibling", "atmos.yaml"),
			basePath: filepath.Join(string(filepath.Separator), "a", "b"),
			want:     filepath.ToSlash(filepath.Join(string(filepath.Separator), "a", "sibling", "atmos.yaml")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := relativePathForDisplay(tt.file, tt.basePath)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRelativePathForDisplay_AbsoluteFallback(t *testing.T) {
	// On Windows, filepath.Rel between paths on different volumes/drives
	// returns an absolute-looking result that filepath.IsAbs recognizes,
	// forcing the fallback branch. Simulate the same fallback shape here by
	// asserting the function degrades to the raw file path whenever Rel
	// cannot produce a safe relative path (covered above via the ".." cases);
	// this test additionally pins the contract that the fallback value is
	// always the slash-normalized original file, never an empty string.
	file := filepath.Join(string(filepath.Separator), "unrelated", "atmos.yaml")
	basePath := filepath.Join(string(filepath.Separator), "a", "b")

	got := relativePathForDisplay(file, basePath)
	assert.Equal(t, filepath.ToSlash(file), got)
	assert.NotEmpty(t, got)
}
