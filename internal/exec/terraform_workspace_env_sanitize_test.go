package exec

// terraform_workspace_env_sanitize_test.go verifies that Atmos does not leak
// the user's `TF_CLI_ARGS` parent-process env var into atmos-internal
// terraform/tofu setup subprocesses:
//
//   - `tofu workspace select` (runWorkspaceSetup)
//   - `tofu workspace new`    (createWorkspaceFallback)
//   - `tofu init` auto-init pre-step (executeTerraformInitPhase, when
//     SubCommand ≠ "init")
//
// User-reported regression (see docs/fixes/2026-04-27-tf-cli-args-breaks-workspace-select.md):
// when CI sets `TF_CLI_ARGS=-lock-timeout=10m` to wait on remote state locks,
// `tofu workspace select` fails with "flag provided but not defined: -lock-timeout"
// because OpenTofu prepends `TF_CLI_ARGS` to every subcommand. The user's
// target subcommand (`plan`/`apply`) never runs.
//
// The same bug class applies to `tofu init` for plan/apply-only flags that
// init does not accept (e.g. `-parallelism`, `-refresh`, `-detailed-exitcode`).
// The user's specific report didn't trigger init because `init` accepts
// `-lock-timeout`, but the next user with `-parallelism=4` would hit it.
//
// Strategy: use the test binary (os.Executable) as the "terraform" command.
// TestMain in testmain_test.go honors `_ATMOS_TEST_ENV_DUMP_FILE` by writing
// the subprocess env to that file and exiting 0. We then read the file and
// assert which TF_CLI_ARGS_* keys reached the subprocess.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// readSubprocessEnv reads the env-dump file produced by the test-binary
// subprocess and returns it as a map.
func readSubprocessEnv(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "env dump file must exist: subprocess was not invoked")
	out := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		out[line[:eq]] = line[eq+1:]
	}
	return out
}

// TestRunWorkspaceSetup_StripsTfCliArgs is the regression test for
// "TF_CLI_ARGS leaks into workspace select" — the user-reported issue where
// `TF_CLI_ARGS=-lock-timeout=10m` set at the CI workflow level caused
// `tofu workspace select` to abort with "flag provided but not defined:
// -lock-timeout" before the user's target `tofu plan` could run.
//
// Before the fix: TF_CLI_ARGS reaches the workspace subprocess (regression
// reproduced).  After the fix: TF_CLI_ARGS is stripped; subcommand-scoped
// variants like TF_CLI_ARGS_plan are preserved.
func TestRunWorkspaceSetup_StripsTfCliArgs(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err, "os.Executable() must succeed")

	dumpFile := filepath.Join(t.TempDir(), "env.dump")

	// Set the parent-process env exactly as a CI workflow would:
	// the unscoped TF_CLI_ARGS targets every terraform subcommand
	// (the leak), and the per-subcommand variants target only their named
	// subcommand (must be preserved through the workspace-setup hop because
	// the subprocess will go on to run plan/apply/init eventually).
	t.Setenv("TF_CLI_ARGS", "-lock-timeout=10m")
	t.Setenv("TF_CLI_ARGS_workspace", "-no-color")
	t.Setenv("TF_CLI_ARGS_plan", "-lock-timeout=10m")
	t.Setenv("TF_CLI_ARGS_apply", "-lock-timeout=10m")
	t.Setenv("TF_CLI_ARGS_init", "-upgrade")
	t.Setenv("TF_VAR_region", "us-east-1")
	t.Setenv("TF_LOG", "DEBUG")

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		Command:            exePath,
		SubCommand:         "plan",
		TerraformWorkspace: "test-ws",
		// Empty backend type so workspace setup is not skipped by shouldSkipWorkspaceSetup.
		ComponentBackendType: "",
		// Tell the test-binary subprocess to dump its env and exit 0 — emulates
		// `tofu workspace select` succeeding.
		ComponentEnvList: []string{
			"_ATMOS_TEST_ENV_DUMP_FILE=" + dumpFile,
		},
	}

	componentPath := t.TempDir()
	wsErr := runWorkspaceSetup(&atmosConfig, &info, componentPath)
	require.NoError(t, wsErr, "workspace setup must succeed (test-binary subprocess exits 0)")

	subprocessEnv := readSubprocessEnv(t, dumpFile)

	// Core regression assertion: the unscoped TF_CLI_ARGS must NOT reach a
	// workspace-setup subprocess. This is the actual bug fix.
	_, hasUnscoped := subprocessEnv["TF_CLI_ARGS"]
	assert.False(t, hasUnscoped,
		"TF_CLI_ARGS must NOT leak into `tofu workspace select`; "+
			"OpenTofu prepends it to every subcommand and `workspace select` rejects "+
			"plan/apply-only flags like -lock-timeout. Got value: %q", subprocessEnv["TF_CLI_ARGS"])

	// TF_CLI_ARGS_workspace explicitly targets workspace ops; it has the same
	// failure mode if it carries an unsupported flag, so it is also stripped.
	_, hasWorkspaceVariant := subprocessEnv["TF_CLI_ARGS_workspace"]
	assert.False(t, hasWorkspaceVariant,
		"TF_CLI_ARGS_workspace must NOT reach `tofu workspace select` either; "+
			"its scope is the same and it has no fine-grained sub-subcommand override")

	// Per-subcommand variants for OTHER subcommands must be preserved: they
	// don't affect `workspace select` (OpenTofu only applies them to their
	// named subcommand) and the user set them deliberately for plan/apply/init.
	assert.Equal(t, "-lock-timeout=10m", subprocessEnv["TF_CLI_ARGS_plan"],
		"TF_CLI_ARGS_plan must be preserved — only applies to `tofu plan`, not `workspace select`")
	assert.Equal(t, "-lock-timeout=10m", subprocessEnv["TF_CLI_ARGS_apply"],
		"TF_CLI_ARGS_apply must be preserved — only applies to `tofu apply`")
	assert.Equal(t, "-upgrade", subprocessEnv["TF_CLI_ARGS_init"],
		"TF_CLI_ARGS_init must be preserved — only applies to `tofu init`")

	// Unrelated TF_* vars must pass through. They are not flag-injection vectors
	// and are load-bearing for terraform's own behavior.
	assert.Equal(t, "us-east-1", subprocessEnv["TF_VAR_region"],
		"TF_VAR_* must pass through; not a CLI args flag")
	assert.Equal(t, "DEBUG", subprocessEnv["TF_LOG"],
		"TF_LOG must pass through; not a CLI args flag")
}

// TestCreateWorkspaceFallback_StripsTfCliArgs covers the workspace-new fallback
// path. When `workspace select` fails with exit code 1 (workspace doesn't
// exist), Atmos calls `workspace new`, which is just as vulnerable to the
// TF_CLI_ARGS leak.
//
// Approach: drive createWorkspaceFallback directly with a test-binary
// subprocess that dumps env and exits 0.
func TestCreateWorkspaceFallback_StripsTfCliArgs(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err, "os.Executable() must succeed")

	dumpFile := filepath.Join(t.TempDir(), "env.dump")

	t.Setenv("TF_CLI_ARGS", "-lock-timeout=10m")
	t.Setenv("TF_CLI_ARGS_apply", "-lock-timeout=10m")

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		Command:            exePath,
		SubCommand:         "apply",
		TerraformWorkspace: "new-ws",
		ComponentEnvList: []string{
			"_ATMOS_TEST_ENV_DUMP_FILE=" + dumpFile,
		},
	}

	componentPath := t.TempDir()
	wsErr := createWorkspaceFallback(&atmosConfig, &info, componentPath)
	require.NoError(t, wsErr, "workspace new fallback must succeed (subprocess exits 0)")

	subprocessEnv := readSubprocessEnv(t, dumpFile)

	_, hasUnscoped := subprocessEnv["TF_CLI_ARGS"]
	assert.False(t, hasUnscoped,
		"TF_CLI_ARGS must NOT leak into `tofu workspace new` either")

	assert.Equal(t, "-lock-timeout=10m", subprocessEnv["TF_CLI_ARGS_apply"],
		"per-subcommand variants must still be preserved on the fallback path")
}

// TestExecuteTerraformInitPhase_StripsTfCliArgs covers the auto-init pre-step.
// Atmos invokes `tofu init` automatically before plan/apply (and other state
// subcommands) when --skip-init is not set.  This is a setup subprocess the
// user did not write, so it must not inherit unscoped `TF_CLI_ARGS` flags
// that init does not accept.
//
// The user's original report used `-lock-timeout=10m` (which init accepts, so
// init didn't crash for them) — but `-parallelism=4`, `-refresh=false`,
// `-detailed-exitcode`, `-replace=…`, `-target=…`, `-out=…` etc. are all
// plan/apply-only flags users commonly set in `TF_CLI_ARGS` that would crash
// init. This test uses `-parallelism=4` as a representative non-`-lock-timeout`
// case to guard against the next user hitting that variant.
func TestExecuteTerraformInitPhase_StripsTfCliArgs(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err, "os.Executable() must succeed")

	dumpFile := filepath.Join(t.TempDir(), "env.dump")

	// `-parallelism=4` is a plan/apply-only flag; `tofu init` rejects it.
	t.Setenv("TF_CLI_ARGS", "-parallelism=4")
	// `TF_CLI_ARGS_init` is the user's deliberate per-subcommand override.
	// It must be preserved through the auto-init pre-step.
	t.Setenv("TF_CLI_ARGS_init", "-upgrade")
	// Per-subcommand variants for OTHER subcommands must also be preserved.
	t.Setenv("TF_CLI_ARGS_plan", "-parallelism=4")
	t.Setenv("TF_VAR_region", "us-east-1")

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		Command:    exePath,
		SubCommand: "plan", // not "init" — this triggers the auto-init pre-step path
		ComponentEnvList: []string{
			"_ATMOS_TEST_ENV_DUMP_FILE=" + dumpFile,
		},
	}

	componentPath := t.TempDir()
	// executeTerraformInitPhase calls prepareInitExecution → cleanTerraformWorkspace
	// → ExecuteShellCommand for `tofu init`. The test-binary subprocess dumps
	// env and exits 0, so the init step "succeeds" and we can inspect what
	// the subprocess actually saw.
	_, initErr := executeTerraformInitPhase(&atmosConfig, &info, componentPath, "")
	require.NoError(t, initErr, "init phase must succeed (test-binary subprocess exits 0)")

	subprocessEnv := readSubprocessEnv(t, dumpFile)

	// Core regression assertion: unscoped TF_CLI_ARGS must NOT reach the
	// auto-init subprocess.  Same bug class as the workspace fix.
	_, hasUnscoped := subprocessEnv["TF_CLI_ARGS"]
	assert.False(t, hasUnscoped,
		"TF_CLI_ARGS must NOT leak into the auto-`tofu init` pre-step; "+
			"plan/apply-only flags like -parallelism crash init. "+
			"Got value: %q", subprocessEnv["TF_CLI_ARGS"])

	// Per-subcommand variants must be preserved.  `TF_CLI_ARGS_init` is
	// especially important — that's how a user who actually wants `-upgrade`
	// on every init expresses it.
	assert.Equal(t, "-upgrade", subprocessEnv["TF_CLI_ARGS_init"],
		"TF_CLI_ARGS_init must be preserved — it is the user-intentional "+
			"per-subcommand variant that targets init specifically")
	assert.Equal(t, "-parallelism=4", subprocessEnv["TF_CLI_ARGS_plan"],
		"TF_CLI_ARGS_plan must be preserved — only applies to `tofu plan`, "+
			"will not affect the init subprocess")

	// Unrelated env vars pass through.
	assert.Equal(t, "us-east-1", subprocessEnv["TF_VAR_region"],
		"TF_VAR_* must pass through; not a CLI args flag")
}
