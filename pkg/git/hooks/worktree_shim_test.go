package hooks

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

const shimProbeEnv = "_ATMOS_TEST_SHIM_PROBE"

type shimProbe struct {
	Binary string
	Args   []string
	Input  string
}

func runShimProbe() int {
	binary, err := os.Executable()
	if err != nil {
		return 2
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return 2
	}
	if err := json.NewEncoder(os.Stdout).Encode(shimProbe{binary, os.Args[1:], string(input)}); err != nil {
		return 2
	}
	if os.Getenv("_ATMOS_TEST_SHIM_FAIL") == "1" {
		return 7
	}
	return 0
}

func copyShimProbe(t *testing.T, path string) {
	t.Helper()
	binary, err := os.Executable()
	require.NoError(t, err)
	data, err := os.ReadFile(binary)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o755))
}

func assertShimBinary(t *testing.T, repo, binary string, env []string, fail bool) {
	t.Helper()
	args := []string{"git", "hooks", "run", "pre-commit", "argument with spaces", "quoted'arg"}
	cmd := exec.Command("git", "hook", "run", "pre-commit", "--", args[4], args[5])
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), env...)
	if fail {
		cmd.Env = append(cmd.Env, "_ATMOS_TEST_SHIM_FAIL=1")
	}
	cmd.Stdin = strings.NewReader("hook input\n")
	out, err := cmd.CombinedOutput()
	if fail {
		var exit *exec.ExitError
		require.ErrorAs(t, err, &exit)
		require.Equal(t, 7, exit.ExitCode())
	} else {
		require.NoError(t, err)
	}
	var got shimProbe
	require.NoError(t, json.Unmarshal(out, &got), string(out))
	want, err := filepath.EvalSymlinks(binary)
	require.NoError(t, err)
	actual, err := filepath.EvalSymlinks(got.Binary)
	require.NoError(t, err)
	assert.Equal(t, want, actual)
	assert.Equal(t, args, got.Args)
	// Git hook run intentionally closes stdin unless --to-stdin is requested.
	assert.Empty(t, got.Input)
}

func TestSharedShimSelectsCurrentWorktree(t *testing.T) {
	root := initTempRepo(t)
	linked := filepath.Join(t.TempDir(), "worktree with 'quotes")
	out, err := gitIn(t, root, nil, "worktree", "add", "-b", "linked", linked)
	require.NoError(t, err, out)
	name := "atmos"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	rootBinary := filepath.Join(root, "build", name)
	linkedBinary := filepath.Join(linked, "build", name)
	systemBinary := filepath.Join(t.TempDir(), name)
	for _, path := range []string{rootBinary, linkedBinary, systemBinary} {
		copyShimProbe(t, path)
	}
	original := installedAtmosPath
	t.Cleanup(func() { installedAtmosPath = original })
	installedAtmosPath = func() string { return filepath.ToSlash(rootBinary) }
	t.Chdir(root)
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {Command: "unused"}}}
	hook := filepath.Join(repoHooksDir(root), "pre-commit")
	// An existing Atmos-managed launcher must upgrade without --force.
	legacy := "#!/bin/sh\n" + ShimMarker + "\natmos_bin=" + shellSingleQuote(filepath.ToSlash(rootBinary)) + "\n"
	require.NoError(t, os.WriteFile(hook, []byte(legacy), 0o755))
	require.NoError(t, Install(t.Context(), cfg, nil, false))
	before, err := os.ReadFile(hook)
	require.NoError(t, err)
	assert.NotContains(t, string(before), filepath.ToSlash(rootBinary))
	env := []string{shimProbeEnv + "=1", "PATH=" + filepath.Dir(systemBinary) + string(os.PathListSeparator) + os.Getenv("PATH")}
	t.Run("each worktree uses its own build", func(t *testing.T) {
		assertShimBinary(t, root, rootBinary, env, false)
		assertShimBinary(t, linked, linkedBinary, env, false)
		assertShimBinary(t, linked, linkedBinary, env, true)
	})
	t.Run("reinstall from another worktree is identical", func(t *testing.T) {
		t.Chdir(linked)
		installedAtmosPath = func() string { return filepath.ToSlash(linkedBinary) }
		require.NoError(t, Install(t.Context(), cfg, nil, false))
		after, err := os.ReadFile(hook)
		require.NoError(t, err)
		assert.Equal(t, string(before), string(after))
	})
	t.Run("missing local build uses system instead of another worktree", func(t *testing.T) {
		require.NoError(t, os.Remove(linkedBinary))
		assertShimBinary(t, linked, systemBinary, env, false)
		assertShimBinary(t, root, rootBinary, env, false)
	})
	t.Run("nonexecutable local build uses system", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows does not use Unix executable bits")
		}
		require.NoError(t, os.WriteFile(linkedBinary, []byte("not executable"), 0o644))
		assertShimBinary(t, linked, systemBinary, env, false)
		require.NoError(t, os.Remove(linkedBinary))
	})
	t.Run("directory is not a build", func(t *testing.T) {
		require.NoError(t, os.Mkdir(linkedBinary, 0o755))
		assertShimBinary(t, linked, systemBinary, env, false)
		require.NoError(t, os.Remove(linkedBinary))
	})
	t.Run("exe build is supported", func(t *testing.T) {
		exeBuild := filepath.Join(linked, "build", "atmos.exe")
		copyShimProbe(t, exeBuild)
		assertShimBinary(t, linked, exeBuild, env, false)
	})
}

func TestShimDoesNotPinDevelopmentBuilds(t *testing.T) {
	for _, name := range []string{"atmos", "atmos.exe"} {
		path := filepath.Join(t.TempDir(), "build", name)
		assert.Equal(t, ShimContent("pre-commit"), ShimContentFor("pre-commit", filepath.ToSlash(path)))
	}
}
