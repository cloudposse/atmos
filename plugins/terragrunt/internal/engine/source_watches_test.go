package engine

import (
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

func TestInputWatchesDoNotEvaluateInputs(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "live", "app", "terragrunt.hcl")
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, afero.WriteFile(fs, path, []byte(`
include "root" { path = find_in_parent_folders("root.hcl") }
inputs = {
  optional = fileexists("optional.json") ? jsondecode(file("optional.json")) : {}
  members = fileset("config", "*.yaml")
  template = templatefile("config.tpl", {})
  secret = sops_decrypt_file("secret.enc.yaml")
  command = run_cmd("must-not-run")
  unrelated = upper("text")
}
`), 0o600))
	result := &model.Resolution{Files: []string{path, filepath.Join(root, "data.json")}}
	require.NoError(t, trackInputSources(fs, root, result))
	for _, name := range []string{"optional.json", "config.tpl", "secret.enc.yaml"} {
		require.Contains(t, result.Files, filepath.Join(filepath.Dir(path), name))
	}
	require.Equal(t, []string{filepath.Join(filepath.Dir(path), "config")}, result.Folders)
	// Missing optional files and empty glob directories are watched before creation.
	exists, err := afero.Exists(fs, filepath.Join(filepath.Dir(path), "optional.json"))
	require.NoError(t, err)
	require.False(t, exists)
}

func TestInputWatchesConservativelyTrackDynamicPaths(t *testing.T) {
	// Expressions that need the environment, outputs, or missing locals cannot be resolved.
	for _, expression := range []string{`file(local.path)`, `file("${get_env("DIR")}/config.json")`, `file(dependency.net.outputs.path)`, `fileset(local.root, "*.yaml")`, `find_in_parent_folders("optional.hcl")`, `file(null)`, `file(123)`, `file()`} {
		t.Run(expression, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "terragrunt.hcl")
			fs := afero.NewMemMapFs()
			require.NoError(t, fs.MkdirAll(root, 0o700))
			require.NoError(t, afero.WriteFile(fs, path, []byte("inputs = { value = "+expression+" }"), 0o600))
			result := &model.Resolution{Files: []string{path}}
			require.NoError(t, trackInputSources(fs, root, result))
			require.Equal(t, []string{root}, result.Folders)
		})
	}
}

func TestInputWatchesFailuresAndIncludedInputs(t *testing.T) {
	root := t.TempDir()
	fs := afero.NewMemMapFs()
	path := filepath.Join(root, "shared.hcl")
	result := &model.Resolution{Files: []string{path}}
	require.Error(t, trackInputSources(fs, root, result))
	require.NoError(t, fs.MkdirAll(root, 0o700))
	require.NoError(t, afero.WriteFile(fs, path, []byte("inputs = {"), 0o600))
	require.ErrorContains(t, trackInputSources(fs, root, result), "cannot track")
	require.NoError(t, afero.WriteFile(fs, path, []byte(`locals { unrelated = "constant" }`), 0o600))
	require.NoError(t, trackInputSources(fs, root, result))
	require.Empty(t, result.Folders)
	require.NoError(t, afero.WriteFile(fs, path, []byte(`inputs = { value = read_terragrunt_config("shared-input.hcl") }`), 0o600))
	require.NoError(t, trackInputSources(fs, root, result))
	require.Contains(t, result.Files, filepath.Join(root, "shared-input.hcl"))
}
