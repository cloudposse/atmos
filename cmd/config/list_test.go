package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

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
