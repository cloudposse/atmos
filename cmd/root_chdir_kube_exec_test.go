package cmd

// Tests that a kubeconfig exec token command tolerates an ATMOS_CHDIR that
// names a deleted directory (for example, a removed worktree), while every
// other case keeps failing as before (issue #3720, #3675).

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// chdirFallbackCase is one ATMOS_CHDIR/--chdir scenario for both chdir entry points.
type chdirFallbackCase struct {
	name      string
	args      []string
	envTarget func(missing, file string) string
	flag      func(missing string) string
	wantErr   error
}

// chdirFallbackCases lists the scenarios shared by the early and the PersistentPreRun chdir paths.
func chdirFallbackCases() []chdirFallbackCase {
	missingEnv := func(missing, _ string) string { return missing }
	return []chdirFallbackCase{
		{name: "aws eks token falls back", args: []string{"atmos", "aws", "eks", "token", "--cluster-name", "prod", "--region", "us-east-2"}, envTarget: missingEnv},
		{name: "azure aks token falls back", args: []string{"atmos", "azure", "aks", "token", "--cluster-name", "dev"}, envTarget: missingEnv},
		{name: "gcp gke token falls back", args: []string{"atmos", "gcp", "gke", "token", "--identity=dev"}, envTarget: missingEnv},
		{name: "global flag before the token command falls back", args: []string{"atmos", "--logs-level=Debug", "aws", "eks", "token"}, envTarget: missingEnv},
		{name: "global flag with a separate value before the token command falls back", args: []string{"atmos", "--logs-level", "Debug", "aws", "eks", "token"}, envTarget: missingEnv},
		{name: "separate value of a global flag is not a command word", args: []string{"atmos", "--logs-level", "aws", "terraform", "plan"}, envTarget: missingEnv, wantErr: errUtils.ErrWorkdirNotExist},
		{name: "other command still fails", args: []string{"atmos", "terraform", "plan", "vpc"}, envTarget: missingEnv, wantErr: errUtils.ErrWorkdirNotExist},
		{name: "token prefix of another command still fails", args: []string{"atmos", "aws", "eks", "update-kubeconfig"}, envTarget: missingEnv, wantErr: errUtils.ErrWorkdirNotExist},
		{name: "explicit --chdir still fails", args: []string{"atmos", "aws", "eks", "token"}, flag: func(missing string) string { return missing }, wantErr: errUtils.ErrWorkdirNotExist},
		{name: "a file still fails", args: []string{"atmos", "aws", "eks", "token"}, envTarget: func(_, file string) string { return file }, wantErr: errUtils.ErrWorkdirNotExist},
	}
}

// setupChdirFallbackCase prepares os.Args, ATMOS_CHDIR and the warning seam,
// and returns the --chdir value, the missing directory and the recorded warnings.
func setupChdirFallbackCase(t *testing.T, tc chdirFallbackCase) (flag, missing string, warnings *[]string) {
	t.Helper()

	_ = NewTestKit(t)
	chdirProcessed = false
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })
	t.Chdir(t.TempDir())

	missing = filepath.Join(t.TempDir(), "removed-worktree")
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	os.Args = tc.args
	t.Setenv("ATMOS_CHDIR", "")
	if tc.envTarget != nil {
		t.Setenv("ATMOS_CHDIR", tc.envTarget(missing, file))
	}
	if tc.flag != nil {
		flag = tc.flag(missing)
		os.Args = append(append([]string{}, tc.args...), "--chdir", flag)
	}

	recorded := []string{}
	originalWarn := warnMissingKubeExecChdir
	t.Cleanup(func() { warnMissingKubeExecChdir = originalWarn })
	warnMissingKubeExecChdir = func(dir string) { recorded = append(recorded, dir) }
	return flag, missing, &recorded
}

// assertChdirFallback checks the outcome of one scenario.
func assertChdirFallback(t *testing.T, tc chdirFallbackCase, err error, startDir, missing string, warnings []string) {
	t.Helper()

	wd, wdErr := os.Getwd()
	require.NoError(t, wdErr)
	assert.Equal(t, startDir, wd, "the working directory must not change")
	if tc.wantErr != nil {
		require.ErrorIs(t, err, tc.wantErr)
		assert.Empty(t, warnings)
		return
	}
	require.NoError(t, err)
	assert.Equal(t, []string{missing}, warnings, "the fallback must warn once with the missing directory")
	assert.True(t, chdirProcessed, "the fallback must not be retried in PersistentPreRun")
	_, set := os.LookupEnv("ATMOS_CHDIR")
	assert.False(t, set, "the stale ATMOS_CHDIR must not reach later code or child processes")
}

// TestProcessEarlyChdirFlag_MissingKubeExecChdir covers the startup chdir path.
func TestProcessEarlyChdirFlag_MissingKubeExecChdir(t *testing.T) {
	for _, tc := range chdirFallbackCases() {
		t.Run(tc.name, func(t *testing.T) {
			_, missing, warnings := setupChdirFallbackCase(t, tc)
			startDir, err := os.Getwd()
			require.NoError(t, err)

			err = processEarlyChdirFlag()
			assertChdirFallback(t, tc, err, startDir, missing, *warnings)
		})
	}
}

// TestProcessChdirFlag_MissingKubeExecChdir covers the PersistentPreRun chdir path.
func TestProcessChdirFlag_MissingKubeExecChdir(t *testing.T) {
	for _, tc := range chdirFallbackCases() {
		t.Run(tc.name, func(t *testing.T) {
			flag, missing, warnings := setupChdirFallbackCase(t, tc)
			startDir, err := os.Getwd()
			require.NoError(t, err)

			cmd := &cobra.Command{Use: "token"}
			cmd.Flags().String("chdir", "", "")
			if flag != "" {
				require.NoError(t, cmd.Flags().Set("chdir", flag))
			}
			err = processChdirFlag(cmd)
			assertChdirFallback(t, tc, err, startDir, missing, *warnings)
		})
	}
}

// TestIsKubeExecTokenInvocation covers the command detection directly.
func TestIsKubeExecTokenInvocation(t *testing.T) {
	assert.True(t, isKubeExecTokenInvocation([]string{"aws", "eks", "token"}))
	assert.True(t, isKubeExecTokenInvocation([]string{"-C=/x", "gcp", "gke", "token", "extra"}))
	assert.False(t, isKubeExecTokenInvocation([]string{"aws", "eks"}))
	assert.False(t, isKubeExecTokenInvocation([]string{"aws", "ecr", "token"}))
	assert.False(t, isKubeExecTokenInvocation([]string{"--", "aws", "eks", "token"}), "arguments after -- are not the command")
	assert.False(t, isKubeExecTokenInvocation(nil))
}

// TestMissingKubeExecChdirMessage verifies that the warning names the missing
// directory and tells the user how to repair the kubeconfig.
func TestMissingKubeExecChdirMessage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "removed-worktree")

	message := missingKubeExecChdirMessage(dir)
	assert.Contains(t, message, dir)
	assert.Contains(t, message, "runs from the current directory")
	assert.Contains(t, message, "atmos auth login")
}
