package hooks

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/git/sandbox"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestInstallPreservesConfiguredHooksPathEvenWithForce(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "force"}[force], func(t *testing.T) {
			root := initTempRepo(t)
			dir := t.TempDir()
			path := filepath.Join(dir, "pre-commit")
			original := []byte("#!/bin/sh\nexisting manager\n")
			require.NoError(t, os.WriteFile(path, original, 0o755))
			cmd := exec.Command("git", "config", "core.hooksPath", dir)
			cmd.Dir = root
			cmd.Env = sandbox.CleanEnvironment(os.Environ())
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, string(output))
			t.Chdir(root)
			cfg := &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{
				"pre-commit": {Steps: schema.Tasks{{Name: "check", Type: "shell", Command: "echo checked"}}},
			}}
			require.ErrorIs(t, Install(t.Context(), cfg, nil, force), errUtils.ErrInvalidConfig)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, original, data)
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Len(t, entries, 1)
		})
	}
}

func TestHooksPathAllowsEmptySettingAndPropagatesGitFailure(t *testing.T) {
	root := initTempRepo(t)
	t.Chdir(root)
	cmd := exec.Command("git", "config", "core.hooksPath", "")
	cmd.Dir = root
	cmd.Env = sandbox.CleanEnvironment(os.Environ())
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	require.NoError(t, validateHooksPath(t.Context()))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.Error(t, validateHooksPath(ctx))
}

func TestInstallRejectsUnreadableHookAndUnavailableBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pre-commit")
	require.NoError(t, os.Mkdir(path, 0o755))
	unchanged, err := prepareExistingHook(path, "shim", true)
	require.ErrorContains(t, err, "checking existing hook")
	require.False(t, unchanged)
	require.Error(t, installHook(dir, "pre-commit", true))
	require.Error(t, backupPreCommitHook(filepath.Join(dir, "missing", "pre-commit"), []byte("original")))
	require.DirExists(t, path)
}

func TestInstallValidationBeforeFilesystemChanges(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, Install(t.Context(), nil, nil, false))
	require.Error(t, Install(t.Context(), nil, []string{"pre-commit"}, false))
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	require.Empty(t, entries)
}
