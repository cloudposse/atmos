package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/generic"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/plugin/releaseplan"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPluginReleasePlanCommittedSources(t *testing.T) {
	_ = NewTestKit(t)
	config := pluginCoverageProject(t)
	directory, head := pluginReleasePlanRepository(t)
	// An invalid working manifest must not change a plan made from committed objects.
	require.NoError(t, os.WriteFile(filepath.Join(directory, "packages", "example", "plugin.yaml"), []byte("not a manifest"), 0o600))
	t.Chdir(filepath.Join(directory, "packages", "example"))
	output, err := invokePluginCoverage(t, config, "release", "plan", "--base", head, "--format", "json")
	require.NoError(t, err)
	var plan releaseplan.Plan
	require.NoError(t, json.Unmarshal([]byte(output), &plan))
	require.Equal(t, []releaseplan.Entry{{Name: "example", Path: "packages/example", Version: "1.0.0", Source: head, Repository: "oci://registry.example.com/team/example"}}, plan.Packages.Include)
	require.Equal(t, "linux/amd64", plan.Build.Include[0].Platform)
	require.Equal(t, "ubuntu-latest", plan.Build.Include[0].Runner)
	_, err = invokePluginCoverage(t, config, "release", "plan", "--format", "yaml")
	require.ErrorIs(t, err, errUtils.ErrPluginReleasePlan)
	_, err = invokePluginCoverage(t, config, "release", "plan", "--inventory", "missing.yaml")
	require.ErrorIs(t, err, errUtils.ErrPluginReleasePlan)
}

func pluginReleasePlanRepository(t *testing.T) (string, string) {
	t.Helper()
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
	return directory, head.String()
}

func TestPluginReleasePlanCIOutputs(t *testing.T) {
	for _, test := range []struct {
		name        string
		enabled     bool
		output      *bool
		github      bool
		missing     bool
		unwritable  bool
		wantError   error
		wantOutputs bool
	}{
		{name: "GitHub matrices", enabled: true, github: true, wantOutputs: true},
		{name: "CI disabled", github: true},
		{name: "outputs disabled", enabled: true, output: new(false), github: true},
		{name: "local rendering suppressed", enabled: true},
		{name: "invalid inventory", enabled: true, github: true, missing: true, wantError: errUtils.ErrPluginReleasePlan},
		{name: "output write failure", enabled: true, github: true, unwritable: true, wantError: errUtils.ErrCIOutputWriteFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			_ = NewTestKit(t)
			config := pluginCoverageProject(t)
			config.CI = schema.CIConfig{Enabled: test.enabled, Output: schema.CIOutputConfig{Enabled: test.output}}
			directory, head := pluginReleasePlanRepository(t)
			t.Chdir(directory)
			t.Cleanup(ci.SwapRegistryForTest())
			ci.Register(github.NewProvider())
			ci.Register(generic.NewProvider())
			t.Setenv("GITHUB_ACTIONS", "false")
			if test.github {
				t.Setenv("GITHUB_ACTIONS", "true")
			}
			t.Setenv("ATMOS_CI", "false")
			t.Setenv("CI", "false")
			t.Setenv("ATMOS_CI_OUTPUT", "")
			outputFile := filepath.Join(t.TempDir(), "outputs")
			require.NoError(t, os.WriteFile(outputFile, []byte("existing=value\n"), 0o600))
			if test.unwritable {
				outputFile = t.TempDir()
			}
			t.Setenv("GITHUB_OUTPUT", outputFile)
			args := []string{"release", "plan", "--base", head}
			if test.missing {
				args = append(args, "--inventory=missing.yaml")
			}
			var stdout string
			var err error
			_, stderr := captureStdoutStderr(t, func() {
				stdout, err = invokePluginCoverage(t, config, args...)
			})
			require.NotContains(t, stderr, "packages=")
			require.NotContains(t, stderr, "build=")
			if test.wantError != nil {
				require.ErrorIs(t, err, test.wantError)
				require.Empty(t, stdout)
			} else {
				require.NoError(t, err)
				var plan releaseplan.Plan
				require.NoError(t, json.Unmarshal([]byte(stdout), &plan))
				if test.wantOutputs {
					packages, marshalErr := json.Marshal(plan.Packages)
					require.NoError(t, marshalErr)
					build, marshalErr := json.Marshal(plan.Build)
					require.NoError(t, marshalErr)
					contents, readErr := os.ReadFile(outputFile)
					require.NoError(t, readErr)
					require.Equal(t, "existing=value\npackages="+string(packages)+"\nbuild="+string(build)+"\n", string(contents))
					require.Len(t, strings.Split(strings.TrimSpace(string(contents)), "\n"), 3)
				}
			}
			if !test.wantOutputs && !test.unwritable {
				contents, readErr := os.ReadFile(outputFile)
				require.NoError(t, readErr)
				require.Equal(t, "existing=value\n", string(contents))
			}
		})
	}
}

func TestPluginReleasePlanUsesCIBaseAndHonorsOverrides(t *testing.T) {
	for _, test := range []struct {
		name      string
		enabled   bool
		base      string
		github    bool
		wantError bool
	}{
		{name: "generic SHA", enabled: true},
		{name: "no provider", enabled: true},
		{name: "generic ref", enabled: true, base: "branch"},
		{name: "disabled CI ignores invalid base", base: "missing"},
		{name: "explicit base wins", enabled: true, base: "missing"},
		{name: "provider error", enabled: true, github: true, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_ = NewTestKit(t)
			config := pluginCoverageProject(t)
			config.CI.Enabled = test.enabled
			config.CI.Output.Enabled = new(false)
			directory, head := pluginReleasePlanRepository(t)
			t.Chdir(directory)
			t.Cleanup(ci.SwapRegistryForTest())
			if test.name != "no provider" {
				ci.Register(generic.NewProvider())
				ci.Register(github.NewProvider())
			}
			t.Setenv("GITHUB_ACTIONS", "false")
			t.Setenv("ATMOS_CI", "true")
			t.Setenv("ATMOS_CI_OUTPUT", "")
			t.Setenv("GITHUB_EVENT_NAME", "push")
			t.Setenv("GITHUB_EVENT_PATH", filepath.Join(t.TempDir(), "absent.json"))
			base := head
			if test.base != "" {
				base = test.base
			}
			t.Setenv("ATMOS_CI_BASE_REF", base)
			if test.github {
				t.Setenv("GITHUB_ACTIONS", "true")
			}
			if test.name == "generic ref" {
				repository, err := git.PlainOpen(directory)
				require.NoError(t, err)
				require.NoError(t, repository.Storer.SetReference(plumbing.NewHashReference("refs/remotes/origin/branch", plumbing.NewHash(head))))
			}
			args := []string{"release", "plan"}
			if test.name == "explicit base wins" {
				args = append(args, "--base", head)
			}
			output, err := invokePluginCoverage(t, config, args...)
			if test.wantError {
				require.ErrorIs(t, err, errUtils.ErrPluginReleasePlan)
				require.Empty(t, output)
			} else {
				require.NoError(t, err)
				require.Contains(t, output, head)
			}
		})
	}
}
