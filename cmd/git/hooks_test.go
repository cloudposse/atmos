package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ---- helper: create a temporary git repository ----

// initTempRepo creates a new git repository in a temp directory and returns the
// path. The test is skipped when git is not available.
func initTempRepo(t *testing.T) string {
	t.Helper()

	_, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not found in PATH; skipping hooks filesystem test.")
	}

	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", gitTestArgs(args...)...)
		cmd.Dir = dir
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err2 := cmd.CombinedOutput()
		require.NoError(t, err2, "git %s: %s", strings.Join(args, " "), out)
	}

	run("init", "-b", "main")
	run("commit", "--allow-empty", "-m", "init")

	return dir
}

// repoHooksDir returns the .git/hooks absolute path for a repo directory.
func repoHooksDir(repoDir string) string {
	return filepath.Join(repoDir, ".git", "hooks")
}

func assertHookExecutable(t *testing.T, path string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		return
	}

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.Mode()&0o100 != 0, "hook file must be executable")
}

// ---- shimContent tests ----

func TestShimContent(t *testing.T) {
	content := shimContent("pre-commit")
	assert.Contains(t, content, "#!/bin/sh")
	assert.Contains(t, content, atmosShimMarker)
	assert.Contains(t, content, "exec atmos git hooks run pre-commit \"$@\"")
}

func TestShimContent_CommitMsg(t *testing.T) {
	content := shimContent("commit-msg")
	assert.Contains(t, content, "exec atmos git hooks run commit-msg \"$@\"")
}

// ---- install: writes executable shim ----

// TestInstallHook_WritesExecutableShim verifies that installHook writes a shim
// with the correct content and executable permission.
func TestInstallHook_WritesExecutableShim(t *testing.T) {
	dir := t.TempDir()

	err := installHook(dir, "pre-commit", false)
	require.NoError(t, err)

	hookPath := filepath.Join(dir, "pre-commit")

	// File must exist with correct content.
	content, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), atmosShimMarker)
	assert.Contains(t, string(content), "exec atmos git hooks run pre-commit \"$@\"")

	// File must be executable.
	assertHookExecutable(t, hookPath)
}

// ---- install: refuses overwrite without --force ----

func TestInstallHook_RefusesOverwriteWithoutForce(t *testing.T) {
	repoDir := initTempRepo(t)
	dir := repoHooksDir(repoDir)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	hookPath := filepath.Join(dir, "pre-commit")
	userContent := "#!/bin/sh\necho 'user hook'\n"
	require.NoError(t, os.WriteFile(hookPath, []byte(userContent), 0o755))

	err := installHook(dir, "pre-commit", false)
	require.Error(t, err)
	assert.False(t, errors.Is(err, errUtils.ErrGitHookNotConfigured))
	// The original content must be unchanged.
	got, _ := os.ReadFile(hookPath)
	assert.Equal(t, userContent, string(got))
}

// ---- install: --force overwrites user-authored hook ----

func TestInstallHook_ForceOverwritesUserHook(t *testing.T) {
	repoDir := initTempRepo(t)
	dir := repoHooksDir(repoDir)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	hookPath := filepath.Join(dir, "pre-commit")
	require.NoError(t, os.WriteFile(hookPath, []byte("#!/bin/sh\necho user\n"), 0o755))

	err := installHook(dir, "pre-commit", true)
	require.NoError(t, err)

	got, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Contains(t, string(got), atmosShimMarker)
}

// ---- install: idempotent for Atmos-generated files ----

func TestInstallHook_OverwritesAtmosShimWithoutForce(t *testing.T) {
	repoDir := initTempRepo(t)
	dir := repoHooksDir(repoDir)
	require.NoError(t, os.MkdirAll(dir, 0o755))

	hookPath := filepath.Join(dir, "pre-commit")
	// Write an existing Atmos shim.
	require.NoError(t, os.WriteFile(hookPath, []byte(shimContent("pre-commit")), 0o755))

	// Should succeed without --force because the marker is present.
	err := installHook(dir, "pre-commit", false)
	require.NoError(t, err)

	got, err := os.ReadFile(hookPath)
	require.NoError(t, err)
	assert.Contains(t, string(got), atmosShimMarker)
}

// ---- install: creates hooks directory when missing ----

func TestInstallHook_CreatesMissingHooksDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent", "hooks")

	err := installHook(dir, "pre-commit", false)
	require.NoError(t, err)

	hookPath := filepath.Join(dir, "pre-commit")
	_, err = os.Stat(hookPath)
	require.NoError(t, err)
	assertHookExecutable(t, hookPath)
}

func TestInstallHook_RejectsUnsafeHookNames(t *testing.T) {
	unsafeNames := []string{
		"",
		".",
		"..",
		"../outside",
		"/tmp/atmos-hook",
		"nested/hook",
		`nested\hook`,
		"pre commit",
		"pre;commit",
	}

	for _, hookName := range unsafeNames {
		name := hookName
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "hooks")

			err := installHook(dir, hookName, true)
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig), "expected ErrInvalidConfig, got: %v", err)

			_, statErr := os.Stat(dir)
			assert.True(t, os.IsNotExist(statErr), "invalid hook names must be rejected before creating hooksDir")

			_, statErr = os.Stat(filepath.Join(parent, "outside"))
			assert.True(t, os.IsNotExist(statErr), "invalid hook names must not write outside hooksDir")
		})
	}
}

// ---- uninstall: removes only Atmos shims ----

func TestUninstallHook_RemovesAtmosShim(t *testing.T) {
	dir := t.TempDir()
	hookPath := filepath.Join(dir, "pre-commit")
	require.NoError(t, os.WriteFile(hookPath, []byte(shimContent("pre-commit")), 0o755))

	err := uninstallHook(dir, "pre-commit")
	require.NoError(t, err)

	_, err = os.Stat(hookPath)
	assert.True(t, os.IsNotExist(err), "shim file should have been removed")
}

// ---- uninstall: skips user-authored hooks ----

func TestUninstallHook_SkipsUserHook(t *testing.T) {
	dir := t.TempDir()
	hookPath := filepath.Join(dir, "pre-commit")
	userContent := "#!/bin/sh\necho 'custom hook'\n"
	require.NoError(t, os.WriteFile(hookPath, []byte(userContent), 0o755))

	err := uninstallHook(dir, "pre-commit")
	require.NoError(t, err, "should not error on user hook, just warn")

	got, err := os.ReadFile(hookPath)
	require.NoError(t, err, "user hook file must still exist")
	assert.Equal(t, userContent, string(got))
}

// ---- uninstall: no-ops on missing file ----

func TestUninstallHook_MissingFileIsNoop(t *testing.T) {
	dir := t.TempDir()
	err := uninstallHook(dir, "pre-commit")
	require.NoError(t, err)
}

func TestUninstallHook_RejectsUnsafeHookNamesBeforeRemovingFiles(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "hooks")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	outside := filepath.Join(parent, "outside")
	require.NoError(t, os.WriteFile(outside, []byte(shimContent("pre-commit")), 0o755))

	err := uninstallHook(dir, "../outside")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig), "expected ErrInvalidConfig, got: %v", err)

	got, readErr := os.ReadFile(outside)
	require.NoError(t, readErr, "unsafe hook name must not remove outside files")
	assert.Contains(t, string(got), atmosShimMarker)
}

// ---- validateHookNames ----

func TestValidateHookNames_Empty(t *testing.T) {
	err := validateHookNames(nil, nil)
	require.NoError(t, err)
}

func TestValidateHookNames_ConfiguredHook(t *testing.T) {
	cfg := &schema.GitConfig{
		Hooks: map[string]schema.GitHookEntry{
			"pre-commit": {Command: "atmos workflow pre-commit"},
		},
	}
	err := validateHookNames([]string{"pre-commit"}, cfg)
	require.NoError(t, err)
}

func TestValidateHookNames_UnconfiguredHook(t *testing.T) {
	cfg := &schema.GitConfig{
		Hooks: map[string]schema.GitHookEntry{
			"pre-commit": {Command: "atmos workflow pre-commit"},
		},
	}
	err := validateHookNames([]string{"commit-msg"}, cfg)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitHookNotConfigured))
}

func TestValidateHookNames_NilConfig(t *testing.T) {
	err := validateHookNames([]string{"pre-commit"}, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitHookNotConfigured))
}

// ---- hookNotConfiguredErr ----

func TestHookNotConfiguredErr_ListsConfiguredHooks(t *testing.T) {
	hooks := map[string]schema.GitHookEntry{
		"pre-commit": {Command: "atmos workflow pre-commit"},
		"commit-msg": {Command: "atmos workflow commit-msg -- \"$1\""},
	}
	err := hookNotConfiguredErr("pre-push", hooks)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitHookNotConfigured))
	// The sentinel text must be present in the error message.
	assert.Contains(t, err.Error(), "git hook not configured")
}

func TestHookNotConfiguredErr_NoHooksConfigured(t *testing.T) {
	err := hookNotConfiguredErr("pre-commit", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitHookNotConfigured))
}

// ---- runHooksRun: missing config → ErrGitHookNotConfigured with exit code 2 ----

func TestRunHooksRun_HookNotConfigured(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })

	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Hooks: map[string]schema.GitHookEntry{
				"pre-commit": {Command: "atmos workflow pre-commit"},
			},
		},
	}

	err := runHooksRun("commit-msg", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitHookNotConfigured))
}

func TestRunHooksRun_NilConfig(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })

	atmosConfigPtr = nil

	err := runHooksRun("pre-commit", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitHookNotConfigured))
}

// ---- buildHookCommand ----

func TestBuildHookCommand_NoArgs(t *testing.T) {
	got := buildHookCommand("atmos workflow pre-commit", nil)
	assert.Equal(t, "atmos workflow pre-commit", got)
}

func TestBuildHookCommand_WithArgs(t *testing.T) {
	got := buildHookCommand("atmos workflow commit-msg", []string{".git/COMMIT_EDITMSG"})
	assert.Equal(t, "atmos workflow commit-msg '.git/COMMIT_EDITMSG'", got)
}

func TestBuildHookCommand_QuotesSingleQuoteInArg(t *testing.T) {
	got := buildHookCommand("echo", []string{"it's"})
	assert.Equal(t, "echo 'it'\\''s'", got)
}

func TestBuildHookCommand_MultipleArgs(t *testing.T) {
	got := buildHookCommand("atmos workflow pre-push", []string{"origin", "https://github.com/acme/repo.git"})
	assert.Equal(t, "atmos workflow pre-push 'origin' 'https://github.com/acme/repo.git'", got)
}

// ---- extractHookNameAndArgs ----

func TestExtractHookNameAndArgs_Basic(t *testing.T) {
	name, args := extractHookNameAndArgs([]string{"pre-commit"})
	assert.Equal(t, "pre-commit", name)
	assert.Empty(t, args)
}

func TestExtractHookNameAndArgs_WithArgs(t *testing.T) {
	name, args := extractHookNameAndArgs([]string{"commit-msg", ".git/COMMIT_EDITMSG"})
	assert.Equal(t, "commit-msg", name)
	assert.Equal(t, []string{".git/COMMIT_EDITMSG"}, args)
}

func TestExtractHookNameAndArgs_Empty(t *testing.T) {
	name, args := extractHookNameAndArgs(nil)
	assert.Equal(t, "", name)
	assert.Nil(t, args)
}

func TestExtractHookNameAndArgs_SkipsLeadingFlags(t *testing.T) {
	name, args := extractHookNameAndArgs([]string{"--verbose", "pre-commit", "extra"})
	assert.Equal(t, "pre-commit", name)
	assert.Equal(t, []string{"extra"}, args)
}

// ---- sortedKeys ----

func TestSortedKeys_ReturnsSorted(t *testing.T) {
	m := map[string]schema.GitHookEntry{
		"pre-push":   {Command: "x"},
		"commit-msg": {Command: "y"},
		"pre-commit": {Command: "z"},
	}
	got := sortedKeys(m)
	assert.Equal(t, []string{"commit-msg", "pre-commit", "pre-push"}, got)
}

func TestSortedKeys_EmptyMap(t *testing.T) {
	got := sortedKeys(map[string]schema.GitHookEntry{})
	assert.Empty(t, got)
}

// ---- runHooksRun: stdin forwarding via ShellRunner ----
// ShellRunner calls interp.StdIO(os.Stdin, ...) unconditionally.
// We assert the design: buildHookCommand produces the expected string that
// would be passed to ShellRunner, which inherits os.Stdin.

func TestRunHooksRun_StdinForwardingDesign(t *testing.T) {
	// Stdin-consuming hooks (pre-push, pre-receive) must not receive spurious args.
	cmd := buildHookCommand("cat", []string{})
	assert.Equal(t, "cat", cmd, "stdin-consuming hook must not receive spurious args")

	// Explicit stdin arg must be quoted and forwarded.
	cmd2 := buildHookCommand("cat", []string{"-"})
	assert.Equal(t, "cat '-'", cmd2)
}

// ---- resolveHooksDir: integration using a temp git repo ----
// Must NOT run in parallel because it modifies cwd (global state).

func TestResolveHooksDir_InsideGitRepo(t *testing.T) {
	repoDir := initTempRepo(t)

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	require.NoError(t, os.Chdir(repoDir))

	dir, err := resolveHooksDir(t.Context())
	require.NoError(t, err)
	assert.NotEmpty(t, dir)
	// Must end with "hooks".
	assert.Equal(t, "hooks", filepath.Base(dir))
}

func TestResolveHooksDir_OutsideGitRepo(t *testing.T) {
	_, gitErr := exec.LookPath("git")
	if gitErr != nil {
		t.Skip("git not found in PATH.")
	}

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	// Change to a non-git directory (os.TempDir is never a git repo).
	require.NoError(t, os.Chdir(os.TempDir()))

	_, err = resolveHooksDir(t.Context())
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrNotInGitRepository))
}

// TestWarnIfHooksPathSet_NoHooksPath verifies that no warning is emitted when
// core.hooksPath is not set.
// Must NOT run in parallel because it modifies cwd.
func TestWarnIfHooksPathSet_NoHooksPath(t *testing.T) {
	repoDir := initTempRepo(t)

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	require.NoError(t, os.Chdir(repoDir))

	// Should not panic or error when core.hooksPath is not set.
	warnIfHooksPathSet(t.Context())
}

// ---- runHooksInstall and runHooksUninstall integration: using temp git repo ----
// Must NOT run in parallel because they modify cwd.

func TestRunHooksInstall_AllConfigured(t *testing.T) {
	repoDir := initTempRepo(t)

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	require.NoError(t, os.Chdir(repoDir))

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Hooks: map[string]schema.GitHookEntry{
				"pre-commit": {Command: "atmos workflow pre-commit"},
				"commit-msg": {Command: "atmos workflow commit-msg -- \"$1\""},
			},
		},
	}

	err = runHooksInstall(t.Context(), nil, false)
	require.NoError(t, err)

	// Both shims must exist.
	for _, name := range []string{"pre-commit", "commit-msg"} {
		p := filepath.Join(repoDir, ".git", "hooks", name)
		content, readErr := os.ReadFile(p)
		require.NoError(t, readErr, "expected shim %s to exist", name)
		assert.Contains(t, string(content), atmosShimMarker)
	}
}

func TestRunHooksUninstall_RemovesAtmosShims(t *testing.T) {
	repoDir := initTempRepo(t)

	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	require.NoError(t, os.Chdir(repoDir))

	hDir := filepath.Join(repoDir, ".git", "hooks")
	require.NoError(t, os.MkdirAll(hDir, 0o755))

	// Write an Atmos shim.
	hookPath := filepath.Join(hDir, "pre-commit")
	require.NoError(t, os.WriteFile(hookPath, []byte(shimContent("pre-commit")), 0o755))

	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })
	atmosConfigPtr = &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Hooks: map[string]schema.GitHookEntry{
				"pre-commit": {Command: "atmos workflow pre-commit"},
			},
		},
	}

	err = runHooksUninstall(t.Context(), nil)
	require.NoError(t, err)

	_, statErr := os.Stat(hookPath)
	assert.True(t, os.IsNotExist(statErr), "shim should be removed")
}
