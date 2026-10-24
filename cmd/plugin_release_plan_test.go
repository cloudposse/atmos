package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/releaseplan"
)

func TestPluginReleasePlanCommittedSources(t *testing.T) {
	_ = NewTestKit(t)
	config := pluginCoverageProject(t)
	directory := t.TempDir()
	repository, err := git.PlainInit(directory, false)
	require.NoError(t, err)
	files := map[string]string{
		"plugins/releases.yaml": `runners:
  linux/amd64: ubuntu-latest
packages:
  example:
    path: packages/example
    repository: oci://registry.example.com/team/example
`,
		"packages/example/plugin.yaml": `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: team/example
  version: 1.0.0
spec:
  runtime:
    type: native
    protocol: 1
    executable: bin/plugin
  platforms:
    linux/amd64:
      executable: bin/plugin
  compatibility:
    protocol: '>=1 <2'
  capabilities: {}
`,
	}
	for path, content := range files {
		filename := filepath.Join(directory, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o700))
		require.NoError(t, os.WriteFile(filename, []byte(content), 0o600))
	}
	worktree, err := repository.Worktree()
	require.NoError(t, err)
	require.NoError(t, worktree.AddWithOptions(&git.AddOptions{All: true}))
	head, err := worktree.Commit("test release", &git.CommitOptions{Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Unix(1, 0)}})
	require.NoError(t, err)
	// An invalid working manifest must not change a plan made from committed objects.
	require.NoError(t, os.WriteFile(filepath.Join(directory, "packages", "example", "plugin.yaml"), []byte("not a manifest"), 0o600))
	t.Chdir(filepath.Join(directory, "packages", "example"))
	output, err := invokePluginCoverage(t, config, "release", "plan", "--base", head.String(), "--format", "json")
	require.NoError(t, err)
	var plan releaseplan.Plan
	require.NoError(t, json.Unmarshal([]byte(output), &plan))
	require.Equal(t, []releaseplan.Entry{{Name: "example", Path: "packages/example", Version: "1.0.0", Source: head.String(), Repository: "oci://registry.example.com/team/example"}}, plan.Packages.Include)
	require.Equal(t, "linux/amd64", plan.Build.Include[0].Platform)
	require.Equal(t, "ubuntu-latest", plan.Build.Include[0].Runner)
	_, err = invokePluginCoverage(t, config, "release", "plan", "--format", "yaml")
	require.ErrorIs(t, err, errUtils.ErrPluginReleasePlan)
	_, err = invokePluginCoverage(t, config, "release", "plan", "--inventory", "missing.yaml")
	require.ErrorIs(t, err, errUtils.ErrPluginReleasePlan)
}
