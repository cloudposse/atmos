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
			assertTreeDoesNotContain(t, target, "name_pattern")
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
