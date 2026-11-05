package cmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const profilePathManifest = `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: acme/oidc
  version: 0.2.0-dev
spec:
  runtime: {type: native, protocol: 1, executable: bin/plugin}
  compatibility: {protocol: ">=1 <2"}
  capabilities:
    commands:
      about: {description: Local build}
`

// profilePathProject writes a project whose base configuration declares a registry package and whose
// profile sets path for it. The profile name is arbitrary: nothing may infer meaning from it.
func profilePathProject(t *testing.T, profile string) (directory string, environment []string) {
	t.Helper()
	root := t.TempDir()
	directory = filepath.Join(root, "project")
	build := filepath.Join(directory, "build", "corporate-oidc")
	require.NoError(t, os.MkdirAll(build, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(build, "plugin.yaml"), []byte(profilePathManifest), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(directory, "profiles", profile), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "atmos.yaml"), []byte(`base_path: .
profiles:
  base_path: profiles
plugins:
  cache_dir: `+filepath.ToSlash(filepath.Join(root, "plugins"))+`
  packages:
    corporate:
      source: oci://registry.example.com/plugins/oidc
      version: ^0.1.0
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "profiles", profile, "atmos.yaml"), []byte(`plugins:
  packages:
    corporate:
      path: build/corporate-oidc
`), 0o600))
	return directory, pluginTestEnvironment(t, root)
}

func runPluginCLIResult(t *testing.T, directory string, environment []string, arguments ...string) (string, error) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir, command.Env = directory, environment
	output, err := command.CombinedOutput()
	return string(output), err
}

func TestProfileSelectsALocalPathPackage(t *testing.T) {
	_ = NewTestKit(t)
	const profile = "kljasdljkhbaslhkdhlkjsd"
	directory, environment := profilePathProject(t, profile)

	t.Run("without the profile the package is the registry release", func(t *testing.T) {
		output, err := runPluginCLIResult(t, directory, environment, "plugin", "list", "--format=json")
		require.NoError(t, err, output)
		assert.Contains(t, output, `"state": "missing"`)
		assert.NotContains(t, output, `"source": "path"`)
		assert.NotContains(t, output, "0.2.0-dev")
	})

	t.Run("--profile selects the local build", func(t *testing.T) {
		output, err := runPluginCLIResult(t, directory, environment, "plugin", "list", "--profile="+profile, "--format=json")
		require.NoError(t, err, output)
		var rows []map[string]any
		start := strings.Index(output, "[")
		require.GreaterOrEqual(t, start, 0, output)
		require.NoError(t, json.Unmarshal([]byte(output[start:]), &rows), output)
		require.Len(t, rows, 1)
		assert.Equal(t, "path", rows[0]["state"])
		assert.Equal(t, "path", rows[0]["source"])
		assert.Equal(t, "acme/oidc", rows[0]["plugin"])
		assert.Equal(t, "0.2.0-dev", rows[0]["version"])
		resolved, err := filepath.EvalSymlinks(filepath.Join(directory, "build", "corporate-oidc"))
		require.NoError(t, err)
		assert.Equal(t, resolved, rows[0]["path"])

		info, err := runPluginCLIResult(t, directory, environment, "plugin", "info", "corporate", "--profile="+profile)
		require.NoError(t, err, info)
		assert.Contains(t, info, "0.2.0-dev")
		assert.Contains(t, info, "bypassed")
	})

	t.Run("ATMOS_PROFILE selects the same build", func(t *testing.T) {
		output, err := runPluginCLIResult(t, directory, append(environment, "ATMOS_PROFILE="+profile), "plugin", "list")
		require.NoError(t, err, output)
		assert.Contains(t, output, "path")
		assert.Contains(t, output, "0.2.0-dev")
	})

	// The hints (active configuration and profile names) are asserted in pkg/plugin/manage; the helper
	// process prints only the error message.
	t.Run("frozen installs refuse", func(t *testing.T) {
		output, err := runPluginCLIResult(t, directory, environment, "plugin", "install", "--frozen", "--profile="+profile)
		require.Error(t, err)
		assert.Contains(t, output, "plugins.packages.corporate.path is set")

		// Negative: the same frozen install without the profile has no path package to refuse.
		output, err = runPluginCLIResult(t, directory, environment, "plugin", "install", "--frozen")
		require.Error(t, err, "no lock exists yet, which is ordinary drift")
		assert.NotContains(t, output, "path is set")
	})

	t.Run("remove refuses a path that comes from configuration", func(t *testing.T) {
		output, err := runPluginCLIResult(t, directory, environment, "plugin", "remove", "corporate", "--profile="+profile)
		require.Error(t, err)
		assert.Contains(t, output, "plugins.packages.corporate.path")
	})

	t.Run("install skips the path package without fetching or locking it", func(t *testing.T) {
		output, err := runPluginCLIResult(t, directory, environment, "plugin", "install", "--profile="+profile)
		require.NoError(t, err, output)
		assert.Contains(t, output, "local build")
		assert.NoFileExists(t, filepath.Join(directory, ".atmos.plugins.lock.yaml"))
	})

	t.Run("removed development flags are rejected", func(t *testing.T) {
		for _, arguments := range [][]string{{"plugin", "install", "./plugin", "--dev"}, {"plugin", "remove", "corporate", "--dev"}} {
			output, err := runPluginCLIResult(t, directory, environment, arguments...)
			require.Error(t, err, arguments)
			assert.Contains(t, output, "--dev", arguments)
		}
	})
}

// A rule over origin, path and profiles decides whether a path package may activate. The package here has
// no executable, so an admitted activation fails later for that reason and a denial fails with the policy.
func TestPolicyGatesPathPackagesByPathAndProfile(t *testing.T) {
	_ = NewTestKit(t)
	const allowed, other = "sandbox-q7", "alice-9"
	directory, environment := profilePathProject(t, allowed)
	for _, name := range []string{other, "outside"} {
		require.NoError(t, os.MkdirAll(filepath.Join(directory, "profiles", name), 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(directory, "profiles", other, "atmos.yaml"), []byte("plugins:\n  packages:\n    corporate:\n      path: build/corporate-oidc\n"), 0o600))
	scratch := filepath.Join(directory, "scratch", "corporate-oidc")
	require.NoError(t, os.MkdirAll(scratch, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(scratch, "plugin.yaml"), []byte(profilePathManifest), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "profiles", "outside", "atmos.yaml"), []byte("plugins:\n  packages:\n    corporate:\n      path: scratch/corporate-oidc\n"), 0o600))

	policies := filepath.Join(directory, "policy.d")
	require.NoError(t, os.MkdirAll(policies, 0o755))
	document := `apiVersion: atmos/v1
kind: Policy
metadata: {name: local-builds}
spec:
  policies:
    approved-roots:
      on: plugin
      action: deny
      reason: roots
      with:
        dev_roots: ['build/']
      rule: "input.origin == 'development' && !params.dev_roots.exists(r, input.path.startsWith(r))"
    sandbox-profile:
      on: plugin
      action: deny
      reason: profile
      rule: "input.origin == 'development' && !('` + allowed + `' in input.profiles)"
`
	require.NoError(t, os.WriteFile(filepath.Join(policies, "plugins.yaml"), []byte(document), 0o600))
	environment = append(environment, "ATMOS_POLICY_DIR="+policies)

	run := func(profile string) string {
		output, err := runPluginCLIResult(t, directory, environment, "plugin", "run", "corporate", "--profile="+profile, "--")
		require.Error(t, err, "the fixture has no executable, so the run cannot succeed")
		return output
	}
	t.Run("approved root and profile are admitted", func(t *testing.T) {
		output := run(allowed)
		assert.NotContains(t, output, "policy denied", output)
		assert.Contains(t, output, "runs from local path", "the warning precedes the failure")
	})
	t.Run("other profile name is denied", func(t *testing.T) {
		assert.Contains(t, run(other), "policy denied")
	})
	t.Run("directory outside the approved root is denied", func(t *testing.T) {
		assert.Contains(t, run("outside"), "policy denied")
	})
	t.Run("ATMOS_PROFILE is honored", func(t *testing.T) {
		output, err := runPluginCLIResult(t, directory, append(environment, "ATMOS_PROFILE="+other), "plugin", "run", "corporate", "--")
		require.Error(t, err)
		assert.Contains(t, output, "policy denied")
		output, err = runPluginCLIResult(t, directory, append(environment, "ATMOS_PROFILE="+allowed), "plugin", "run", "corporate", "--")
		require.Error(t, err)
		assert.NotContains(t, output, "policy denied")
	})
}
