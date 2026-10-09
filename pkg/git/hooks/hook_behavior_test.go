package hooks

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// These tests change the working directory, so they must not run in parallel.

func shellStep(name, command string) schema.Task {
	return schema.Task{Name: name, Type: "shell", Command: command}
}

func TestRunSteps_EnvStepValuesReachLaterSteps(t *testing.T) {
	t.Setenv("WP5_PROCESS_VAR", "from-process")
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {Steps: schema.Tasks{
		{Name: "setenv", Type: "env", Vars: map[string]string{"SHARED_VAR": "from-env-step"}},
		shellStep("shell-sees", `echo "child=$SHARED_VAR template={{ .env.SHARED_VAR }} process=$WP5_PROCESS_VAR path_set=${PATH:+yes}"`),
	}}}}

	var stdout, stderr bytes.Buffer
	err := Run(cfg, "pre-commit", nil, WithOutputWriters(&stdout, &stderr))
	require.NoError(t, err, stderr.String())
	out := stdout.String()
	assert.Contains(t, out, "child=from-env-step")
	assert.Contains(t, out, "template=from-env-step")
	assert.Contains(t, out, "process=from-process")
	assert.Contains(t, out, "path_set=yes", "the process PATH must survive an env step")
}

func TestRunSteps_AtmosConfigGlobalEnvSeedsSteps(t *testing.T) {
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {Steps: schema.Tasks{
		shellStep("show", `echo "global=$WP5_GLOBAL_ENV"`),
	}}}}

	var stdout, stderr bytes.Buffer
	err := Run(cfg, "pre-commit", nil,
		WithAtmosConfig(&schema.AtmosConfiguration{Env: map[string]string{"WP5_GLOBAL_ENV": "from-atmos-yaml"}}),
		WithOutputWriters(&stdout, &stderr))
	require.NoError(t, err, stderr.String())
	assert.Contains(t, stdout.String(), "global=from-atmos-yaml")
}

func TestRun_UsesRepositoryRootFromSubdirectory(t *testing.T) {
	repoDir := initTempRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "root-marker.txt"), []byte("x"), 0o644))
	sub := filepath.Join(repoDir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	t.Chdir(sub)

	t.Run("steps hook", func(t *testing.T) {
		cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {Steps: schema.Tasks{
			{Name: "probe", Type: "script", Interpreter: "starlark", Script: `print("marker=" + str(fs.exists("root-marker.txt")))`},
		}}}}
		var stdout, stderr bytes.Buffer
		require.NoError(t, Run(cfg, "pre-commit", nil, WithOutputWriters(&stdout, &stderr)), stderr.String())
		assert.Contains(t, stdout.String(), "marker=True")
	})

	t.Run("command hook", func(t *testing.T) {
		cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-push": {Command: `test -f root-marker.txt && echo found > command-ran.txt`}}}
		require.NoError(t, Run(cfg, "pre-push", nil))
		_, err := os.Stat(filepath.Join(repoDir, "command-ran.txt"))
		assert.NoError(t, err, "command hooks run from the repository root")
		_, err = os.Stat(filepath.Join(sub, "command-ran.txt"))
		assert.True(t, os.IsNotExist(err), "the subdirectory must not be the working directory")
	})
}

func TestResolveWorkingDir_FallsBackToCwdOutsideRepository(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Chdir(dir)

	got, err := resolveWorkingDir()
	require.NoError(t, err)
	want, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	gotResolved, err := filepath.EvalSymlinks(got)
	require.NoError(t, err)
	assert.Equal(t, want, gotResolved)
}

func TestUninstall_NoNamesRemovesOrphanAtmosShims(t *testing.T) {
	repoDir := initTempRepo(t)
	t.Chdir(repoDir)
	hooksDir := repoHooksDir(repoDir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))

	for _, name := range []string{"pre-commit", "commit-msg", "pre-push"} {
		require.NoError(t, os.WriteFile(filepath.Join(hooksDir, name), []byte(ShimContent(name)), 0o755))
	}
	userHook := filepath.Join(hooksDir, "post-merge")
	require.NoError(t, os.WriteFile(userHook, []byte("#!/bin/sh\necho mine\n"), 0o755))
	sample := filepath.Join(hooksDir, "pre-rebase.sample")
	require.NoError(t, os.WriteFile(sample, []byte("#!/bin/sh\n# sample\n"), 0o755))

	// Only pre-commit is configured; commit-msg and pre-push are orphans.
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {Command: "true"}}}
	require.NoError(t, Uninstall(t.Context(), cfg, nil))

	for _, name := range []string{"pre-commit", "commit-msg", "pre-push"} {
		_, err := os.Stat(filepath.Join(hooksDir, name))
		assert.True(t, os.IsNotExist(err), "%s shim should be removed", name)
	}
	assert.FileExists(t, userHook, "user-authored hooks are never removed")
	assert.FileExists(t, sample)
}

func TestUninstall_NoNamesWithNoShimsIsNoop(t *testing.T) {
	repoDir := initTempRepo(t)
	t.Chdir(repoDir)
	require.NoError(t, Uninstall(t.Context(), nil, nil))
}

func TestUninstall_ExplicitNames(t *testing.T) {
	repoDir := initTempRepo(t)
	t.Chdir(repoDir)
	hooksDir := repoHooksDir(repoDir)
	require.NoError(t, os.MkdirAll(hooksDir, 0o755))
	orphan := filepath.Join(hooksDir, "pre-push")
	require.NoError(t, os.WriteFile(orphan, []byte(ShimContent("pre-push")), 0o755))
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {Command: "true"}}}

	t.Run("unknown name that is not an installed shim fails like install", func(t *testing.T) {
		err := Uninstall(t.Context(), cfg, []string{"no-such-hook"})
		require.ErrorIs(t, err, errUtils.ErrGitHookNotConfigured)
	})

	t.Run("unknown name fails before anything is removed", func(t *testing.T) {
		err := Uninstall(t.Context(), cfg, []string{"pre-push", "no-such-hook"})
		require.ErrorIs(t, err, errUtils.ErrGitHookNotConfigured)
		assert.FileExists(t, orphan)
	})

	t.Run("orphan shim is removable by name", func(t *testing.T) {
		require.NoError(t, Uninstall(t.Context(), cfg, []string{"pre-push"}))
		_, err := os.Stat(orphan)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("configured name with no shim is a no-op", func(t *testing.T) {
		require.NoError(t, Uninstall(t.Context(), cfg, []string{"pre-commit"}))
	})
}

func brokenHookConfig() *schema.GitConfig {
	return &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{
		"pre-commit":   {Steps: schema.Tasks{shellStep("ok", "echo ok")}},
		"broken-both":  {Command: "echo hi", Steps: schema.Tasks{shellStep("a", "echo a")}},
		"broken-typo":  {Steps: schema.Tasks{{Name: "t", Type: "starlark", Script: "print(1)"}}},
		"broken-dup":   {Steps: schema.Tasks{shellStep("same", "echo 1"), shellStep("same", "echo 2")}},
		"broken-needs": {Steps: schema.Tasks{{Name: "w", Type: "shell", Command: "echo w", Needs: []string{"other"}}}},
	}}
}

func TestInstall_RejectsBrokenHooksBeforeWritingAnyShim(t *testing.T) {
	repoDir := initTempRepo(t)
	t.Chdir(repoDir)

	err := Install(t.Context(), brokenHookConfig(), nil, false)
	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
	require.ErrorIs(t, err, errUtils.ErrAutomation)
	var invalid *invalidHooksError
	require.ErrorAs(t, err, &invalid)
	var reported []string
	for _, e := range invalid.errs {
		reported = append(reported, e.Error())
	}
	joined := strings.Join(reported, "\n")
	for _, name := range []string{"broken-both", "broken-typo", "broken-dup", "broken-needs"} {
		assert.Contains(t, joined, `git hook "`+name+`"`, "every broken hook is reported by name")
	}
	assert.NotContains(t, joined, `git hook "pre-commit"`)

	entries, readErr := os.ReadDir(repoHooksDir(repoDir))
	require.NoError(t, readErr)
	for _, e := range entries {
		assert.True(t, strings.HasSuffix(e.Name(), ".sample"), "no shim may be written, found %s", e.Name())
	}
}

func TestInstall_ValidNamedHookIgnoresOtherBrokenHooks(t *testing.T) {
	repoDir := initTempRepo(t)
	t.Chdir(repoDir)

	require.NoError(t, Install(t.Context(), brokenHookConfig(), []string{"pre-commit"}, false))
	assert.FileExists(t, filepath.Join(repoHooksDir(repoDir), "pre-commit"))

	err := Install(t.Context(), brokenHookConfig(), []string{"broken-typo"}, false)
	require.ErrorIs(t, err, errUtils.ErrUnknownStepType)
	assert.NoFileExists(t, filepath.Join(repoHooksDir(repoDir), "broken-typo"))
}

func TestInstall_NonGitHookNameWarnsButInstalls(t *testing.T) {
	repoDir := initTempRepo(t)
	t.Chdir(repoDir)
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"my-custom": {Command: "true"}}}

	require.NoError(t, Install(t.Context(), cfg, nil, false))
	assert.FileExists(t, filepath.Join(repoHooksDir(repoDir), "my-custom"))
	assert.False(t, IsKnownGitHookName("my-custom"))
	for _, name := range []string{"pre-commit", "commit-msg", "pre-push", "post-index-change", "p4-pre-submit"} {
		assert.True(t, IsKnownGitHookName(name), name)
	}
}

func TestInstall_RerunLeavesUnchangedShimAlone(t *testing.T) {
	repoDir := initTempRepo(t)
	t.Chdir(repoDir)
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {Command: "true"}}}
	dest := filepath.Join(repoHooksDir(repoDir), "pre-commit")

	require.NoError(t, Install(t.Context(), cfg, nil, false))
	before, err := os.Stat(dest)
	require.NoError(t, err)

	require.NoError(t, Install(t.Context(), cfg, nil, false))
	after, err := os.Stat(dest)
	require.NoError(t, err)
	assert.True(t, before.ModTime().Equal(after.ModTime()), "an unchanged shim must not be rewritten")
	content, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, ShimContent("pre-commit"), string(content))
}

func TestInstall_RerunRepairsExecutableMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use Unix executable permission bits")
	}
	repoDir := initTempRepo(t)
	t.Chdir(repoDir)
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {Command: "true"}}}
	dest := filepath.Join(repoHooksDir(repoDir), "pre-commit")

	require.NoError(t, Install(t.Context(), cfg, nil, false))
	require.NoError(t, os.Chmod(dest, 0o644))
	before, err := os.Stat(dest)
	require.NoError(t, err)
	require.Zero(t, before.Mode().Perm()&0o111, "the fixture must have lost its executable bits")

	require.NoError(t, Install(t.Context(), cfg, nil, false))
	after, err := os.Stat(dest)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(shimPerm), after.Mode().Perm(), "reinstall restores executable permissions")
	assert.True(t, before.ModTime().Equal(after.ModTime()), "repairing permissions must not rewrite the shim")
	content, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, ShimContent("pre-commit"), string(content))
}

func TestValidateHook(t *testing.T) {
	cfg := brokenHookConfig()
	require.NoError(t, ValidateHook("pre-commit", cfg.Hooks["pre-commit"]))
	require.NoError(t, ValidateHook("cmd", schema.GitHookEntry{Command: "echo hi"}))
	for _, name := range []string{"broken-both", "broken-typo", "broken-dup", "broken-needs"} {
		err := ValidateHook(name, cfg.Hooks[name])
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), `git hook "`+name+`"`)
	}
	require.ErrorIs(t, ValidateHook("empty", schema.GitHookEntry{}), errUtils.ErrInvalidConfig)
	require.ErrorIs(t, ValidateHook("typo", cfg.Hooks["broken-typo"]), errUtils.ErrUnknownStepType)
	require.ErrorIs(t, ValidateHook("dup", cfg.Hooks["broken-dup"]), errUtils.ErrAutomation)
}

func TestRun_ErrorsCarryHookName(t *testing.T) {
	for name, entry := range map[string]schema.GitHookEntry{
		"empty":   {},
		"both":    {Command: "echo", Steps: schema.Tasks{shellStep("a", "echo")}},
		"dup":     {Steps: schema.Tasks{shellStep("same", "echo"), shellStep("same", "echo")}},
		"typo":    {Steps: schema.Tasks{{Name: "t", Type: "starlark"}}},
		"runfail": {Steps: schema.Tasks{shellStep("boom", "exit 3")}},
		"cmdfail": {Command: "exit 3"},
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			err := Run(&schema.GitConfig{Hooks: map[string]schema.GitHookEntry{name: entry}}, name, nil, WithOutputWriters(&out, &out))
			require.Error(t, err)
			assert.Contains(t, err.Error(), `git hook "`+name+`"`)
		})
	}
}

func TestRun_StepTimeoutIsReportedAsStepTimeout(t *testing.T) {
	cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {Steps: schema.Tasks{
		{Name: "bounded", Type: "script", Interpreter: "starlark", Timeout: "1ms", Script: "for i in range(1000000000):\n    pass"},
	}}}}
	var out bytes.Buffer
	err := Run(cfg, "pre-commit", nil, WithOutputWriters(&out, &out))
	require.ErrorIs(t, err, errUtils.ErrStepTimeout)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), `git hook "pre-commit"`)
	assert.Contains(t, err.Error(), "bounded")
}

func TestMarkStepTimeout_LeavesOtherErrorsAlone(t *testing.T) {
	tasks := schema.Tasks{{Name: "bounded", Type: "shell", Timeout: "1s"}}
	deadline := errors.New(`step "bounded": ` + context.DeadlineExceeded.Error())
	wrapped := errors.Join(deadline, context.DeadlineExceeded)

	t.Run("caller cancellation is not a step timeout", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		assert.NotErrorIs(t, markStepTimeout(ctx, tasks, wrapped), errUtils.ErrStepTimeout)
	})
	t.Run("unrelated error is unchanged", func(t *testing.T) {
		other := errors.New("boom")
		assert.Equal(t, other, markStepTimeout(t.Context(), tasks, other))
	})
	t.Run("deadline from a step without a timeout is unchanged", func(t *testing.T) {
		noTimeout := schema.Tasks{{Name: "bounded", Type: "shell"}}
		assert.NotErrorIs(t, markStepTimeout(t.Context(), noTimeout, wrapped), errUtils.ErrStepTimeout)
	})
	t.Run("step timeout is attributed to the failing step", func(t *testing.T) {
		assert.ErrorIs(t, markStepTimeout(t.Context(), tasks, wrapped), errUtils.ErrStepTimeout)
	})
}

func TestHookEnvironmentPreservesRelativeGitPaths(t *testing.T) {
	repoDir := initTempRepo(t)
	sub := filepath.Join(repoDir, "sub")
	require.NoError(t, os.Mkdir(sub, 0o755))
	t.Chdir(sub)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	t.Setenv("GIT_DIR", filepath.Join("..", ".git"))
	t.Setenv("GIT_WORK_TREE", "..")
	t.Setenv("GIT_INDEX_FILE", filepath.Join("..", ".git", "custom-index"))
	t.Setenv("OTHER_RELATIVE_PATH", "unchanged")

	dir, env, err := resolveHookEnvironment()
	require.NoError(t, err)
	wantRoot, err := filepath.EvalSymlinks(filepath.Dir(cwd))
	require.NoError(t, err)
	assert.Equal(t, wantRoot, dir)
	for _, entry := range []string{
		"GIT_DIR=" + filepath.Join(cwd, "..", ".git"),
		"GIT_WORK_TREE=" + filepath.Dir(cwd),
		"GIT_INDEX_FILE=" + filepath.Join(cwd, "..", ".git", "custom-index"),
		"OTHER_RELATIVE_PATH=unchanged",
	} {
		assert.Contains(t, env, entry)
	}

	t.Run("steps receive corrected Git environment", func(t *testing.T) {
		cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {Steps: schema.Tasks{
			{Name: "probe", Type: "script", Interpreter: "starlark", Script: `print(exec.run(["git", "rev-parse", "--absolute-git-dir"], output = "capture").stdout.strip())`},
		}}}}
		var stdout, stderr bytes.Buffer
		require.NoError(t, Run(cfg, "pre-commit", nil, WithOutputWriters(&stdout, &stderr)), stderr.String())
		assert.Equal(t, filepath.ToSlash(filepath.Join(wantRoot, ".git")), filepath.ToSlash(strings.TrimSpace(stdout.String())))
	})
	t.Run("commands receive corrected Git environment", func(t *testing.T) {
		cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {
			Command: `git rev-parse --absolute-git-dir > git-path.txt`,
		}}}
		require.NoError(t, Run(cfg, "pre-commit", nil))
		result, err := os.ReadFile(filepath.Join(repoDir, "git-path.txt"))
		require.NoError(t, err)
		assert.Equal(t, filepath.ToSlash(filepath.Join(wantRoot, ".git")), filepath.ToSlash(strings.TrimSpace(string(result))))
	})
}

func TestHookEnvironmentKeepsAbsoluteAndEmptyGitPaths(t *testing.T) {
	t.Chdir(t.TempDir())
	absolute := filepath.Join(t.TempDir(), "index")
	t.Setenv("GIT_INDEX_FILE", absolute)
	t.Setenv("GIT_DIR", "")
	t.Setenv("GIT_WORK_TREE", "")
	_, env, err := resolveHookEnvironment()
	require.NoError(t, err)
	assert.Contains(t, env, "GIT_INDEX_FILE="+absolute)
	assert.Contains(t, env, "GIT_DIR=")
	assert.Contains(t, env, "GIT_WORK_TREE=")
}
