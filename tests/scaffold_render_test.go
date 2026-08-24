package tests

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestScaffoldTemplatesRenderValidateAndAvoidNamePattern(t *testing.T) {
	ensureAtmosRunner(t)

	sourceOverride := filepath.Join(repoRoot, "examples", "scaffolds")
	cases := []struct {
		name string
		args []string
	}{
		{name: "basic", args: []string{"--set", "project_name=render-basic"}},
		{name: "aws/app", args: []string{"--set", "project_name=render-aws-app"}},
		{name: "aws/landing-zone", args: []string{"--set", "project_name=render-aws-lz"}},
		{name: "gcp/landing-zone", args: []string{"--set", "project_name=render-gcp-lz"}},
		{name: "azure/landing-zone", args: []string{"--set", "project_name=render-az-lz"}},
	}

	for _, tc := range cases {
		t.Run(strings.ReplaceAll(tc.name, "/", "_"), func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "project")
			initArgs := append([]string{"init", tc.name, target, "--no-git"}, tc.args...)
			env := map[string]string{
				"ATMOS_INIT_INTERACTIVE":         "false",
				"ATMOS_SCAFFOLD_SOURCE_OVERRIDE": sourceOverride,
				"XDG_CACHE_HOME":                 filepath.Join(t.TempDir(), ".cache"),
			}
			runAtmosForScaffoldTest(t, "", env, 2*time.Minute, initArgs...)
			runAtmosForScaffoldTest(t, target, map[string]string{
				"ATMOS_CLI_CONFIG_PATH": target,
				"XDG_CACHE_HOME":        filepath.Join(t.TempDir(), ".cache"),
			}, 2*time.Minute, "validate", "stacks")
			readme, err := os.ReadFile(filepath.Join(target, "README.md"))
			require.NoError(t, err)
			projectName := strings.TrimPrefix(tc.args[1], "project_name=")
			require.Contains(t, string(readme), "# "+projectName)
			require.NotContains(t, string(readme), "[[ .Config.project_name ]]", "generated README must not leak scaffold template syntax")
			assertTreeDoesNotContain(t, target, "name_pattern")
			// `-i` is the shorthand for `--identity` on `atmos terraform`
			// commands, not `--interactive` -- `-i false` silently disables
			// identity/emulator binding (selects a nonexistent "false"
			// identity) rather than skipping the apply-approval prompt,
			// which is `-auto-approve`. A scaffold's own generated `atmos
			// test` command using `-i false` would break emulator auth for
			// every user who runs it, with no error pointing at the cause.
			assertTreeDoesNotContain(t, target, "-i false")
		})
	}
}

// TestScaffoldTofuToolchainDependencyUsesOpenTofuRegistryName guards against a
// regression where the `terraform_command` scaffold field (which selects the
// invoked binary, "terraform" or "tofu") was reused verbatim as the
// `dependencies.tools` registry key. The toolchain registry has no "tofu"
// entry (that's OpenTofu's binary name, not its aqua package name), so
// selecting "tofu" produced `atmos toolchain install` failures with
// "tool not in registry" the first time a generated project ran `atmos test`.
func TestScaffoldTofuToolchainDependencyUsesOpenTofuRegistryName(t *testing.T) {
	ensureAtmosRunner(t)

	sourceOverride := filepath.Join(repoRoot, "examples", "scaffolds")
	names := []string{"aws/app", "aws/landing-zone", "gcp/landing-zone", "azure/landing-zone"}

	for _, name := range names {
		t.Run(strings.ReplaceAll(name, "/", "_"), func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "project")
			initArgs := []string{
				"init", name, target, "--no-git",
				"--set", "project_name=render-tofu",
				"--set", "terraform_command=tofu",
			}
			if name == "gcp/landing-zone" {
				initArgs = append(initArgs, "--set", "gcp_project=render-tofu")
			}
			env := map[string]string{
				"ATMOS_INIT_INTERACTIVE":         "false",
				"ATMOS_SCAFFOLD_SOURCE_OVERRIDE": sourceOverride,
				"XDG_CACHE_HOME":                 filepath.Join(t.TempDir(), ".cache"),
			}
			runAtmosForScaffoldTest(t, "", env, 2*time.Minute, initArgs...)

			content, err := os.ReadFile(filepath.Join(target, "stacks", "_defaults.yaml"))
			require.NoError(t, err)

			// Parse the rendered YAML rather than matching raw text: the
			// dependencies.tools key is a templated, quoted YAML scalar and
			// its exact quoting style is an implementation detail.
			var rendered struct {
				Dependencies struct {
					Tools map[string]string `yaml:"tools"`
				} `yaml:"dependencies"`
			}
			require.NoError(t, yaml.Unmarshal(content, &rendered))
			_, hasOpentofu := rendered.Dependencies.Tools["opentofu"]
			require.True(t, hasOpentofu,
				"dependencies.tools must use the registry name `opentofu`, not the `tofu` binary name")
			_, hasTofu := rendered.Dependencies.Tools["tofu"]
			require.False(t, hasTofu,
				"dependencies.tools must not use the bare `tofu` binary name as a registry key")
		})
	}
}

func runAtmosForScaffoldTest(t *testing.T, dir string, env map[string]string, timeout time.Duration, args ...string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := atmosRunner.CommandContext(ctx, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = mergeCommandEnv(cmd.Env, env)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.NoErrorf(t, err, "atmos %s failed\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), stdout.String(), stderr.String())
}

func assertTreeDoesNotContain(t *testing.T, root, needle string) {
	t.Helper()

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == ".terraform" {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		require.NotContains(t, string(data), needle, "%s should not contain %q", path, needle)
		return nil
	})
	require.NoError(t, err)
}
