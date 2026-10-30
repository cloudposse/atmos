package terraform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// identityOnlyRuntime can bind plans but cannot resolve backend state.
type identityOnlyRuntime struct {
	schema.ComponentConfigurationRuntime
}

func (identityOnlyRuntime) PlanIdentity(context.Context, *schema.ConfigAndStacksInfo, []schema.ComponentConfigurationSource) (string, error) {
	return "provider-source-options", nil
}

// stateFailureRuntime accepts the plan identity but fails to resolve the backend.
type stateFailureRuntime struct {
	*planRuntimeFixture
	stateErr error
}

func (r stateFailureRuntime) ResolveState(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
	return nil, r.stateErr
}

// recordedPlan records a plan companion and prepares the invocation for a direct apply.
func recordedPlan(t *testing.T) (*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo, *componentExecContext, *planRuntimeFixture, string) {
	t.Helper()
	config, info, execCtx, runtime, path := planFixture(t)
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"plan", "-out=" + path}))
	info.SubCommand = "apply"
	info.UseTerraformPlan = true
	info.PlanFile = path
	return config, info, execCtx, runtime, path
}

func TestRestoreConfigurationPlanRequiresPlanBindingRuntime(t *testing.T) {
	config, info, _, _, _ := recordedPlan(t)
	config.ComponentConfigurations = nil
	prepared, err := restoreConfigurationPlan(t.Context(), config, info)
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
	require.Nil(t, prepared)
}

func TestRestoreConfigurationPlanRequiresStateRuntime(t *testing.T) {
	config, info, _, _, _ := recordedPlan(t)
	config.ComponentConfigurations = identityOnlyRuntime{}
	prepared, err := restoreConfigurationPlan(t.Context(), config, info)
	require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension, "a provider that cannot prove the backend is unchanged must not apply a saved plan")
	require.Nil(t, prepared)
}

func TestRestoreConfigurationPlanBackendResolution(t *testing.T) {
	failure := errors.New("backend unreachable")
	t.Run("resolution error is propagated", func(t *testing.T) {
		config, info, _, runtime, _ := recordedPlan(t)
		config.ComponentConfigurations = stateFailureRuntime{planRuntimeFixture: runtime, stateErr: failure}
		_, err := restoreConfigurationPlan(t.Context(), config, info)
		require.ErrorIs(t, err, failure)
		require.NotErrorIs(t, err, errUtils.ErrPlanVerificationFailed, "an unreachable backend is not a verification mismatch")
	})
	t.Run("missing state is a verification failure", func(t *testing.T) {
		config, info, _, runtime, _ := recordedPlan(t)
		runtime.state = nil
		_, err := restoreConfigurationPlan(t.Context(), config, info)
		require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	})
	t.Run("backend type change is a verification failure", func(t *testing.T) {
		config, info, _, runtime, _ := recordedPlan(t)
		runtime.state.BackendType = "s3"
		_, err := restoreConfigurationPlan(t.Context(), config, info)
		require.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	})
}

func TestRestoreConfigurationPlanReportsUnreadableArtifacts(t *testing.T) {
	t.Run("missing engine binary", func(t *testing.T) {
		config, info, _, _, _ := recordedPlan(t)
		require.NoError(t, os.Remove(info.Command))
		prepared, err := restoreConfigurationPlan(t.Context(), config, info)
		require.ErrorIs(t, err, os.ErrNotExist)
		require.Nil(t, prepared)
	})
	t.Run("symlink added to the reviewed workdir", func(t *testing.T) {
		config, info, execCtx, _, _ := recordedPlan(t)
		if err := os.Symlink(filepath.Join(execCtx.workingDir, "main.tf"), filepath.Join(execCtx.workingDir, "alias.tf")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		prepared, err := restoreConfigurationPlan(t.Context(), config, info)
		require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedExtension)
		require.Nil(t, prepared)
	})
	t.Run("unchanged inputs are accepted", func(t *testing.T) {
		config, info, execCtx, _, path := recordedPlan(t)
		prepared, err := restoreConfigurationPlan(t.Context(), config, info)
		require.NoError(t, err)
		require.Equal(t, execCtx.preparedConfiguration.Backend, prepared.Backend)
		require.Equal(t, path, info.PlanFile)
	})
}

func TestRecordConfigurationPlanFailsWhenWorkdirCannotBeDigested(t *testing.T) {
	config, info, execCtx, _, path := planFixture(t)
	execCtx.workingDir = filepath.Join(t.TempDir(), "absent")
	err := recordConfigurationPlan(t.Context(), config, info, execCtx, []string{"plan", "-out=" + path})
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoFileExists(t, path+configurationPlanSuffix, "no binding may be written for an unverifiable workdir")
}

func TestRecordConfigurationPlanReplacesBindingAtomically(t *testing.T) {
	config, info, execCtx, runtime, path := planFixture(t)
	args := []string{"plan", "-out=" + path}
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, args))
	first, err := os.ReadFile(path + configurationPlanSuffix)
	require.NoError(t, err)
	runtime.identity = "changed provider"
	require.NoError(t, recordConfigurationPlan(t.Context(), config, info, execCtx, args))
	second, err := os.ReadFile(path + configurationPlanSuffix)
	require.NoError(t, err)
	require.NotEqual(t, first, second, "re-planning must replace the earlier binding")
	require.Contains(t, string(second), "changed provider")
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	for _, entry := range entries {
		require.NotContains(t, entry.Name(), ".atmos-plan-", "temporary binding files are cleaned up")
	}
}

func TestConfigurationWorkdirDigestSkipsOnlyNestedMutableDirectories(t *testing.T) {
	root := t.TempDir()
	plan := filepath.Join(root, "reviewed.plan")
	write := func(name string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(name), 0o600))
	}
	write("main.tf")
	baseline, err := configurationWorkdirDigest(root, plan)
	require.NoError(t, err)

	for _, ignored := range []string{".git/HEAD", ".terragrunt-cache/x/main.tf", ".terraform/modules.json", "sub/.terraform/x", ".atmos/metadata.json", ".atmos/metadata.json.lock", "sub/.atmos/source-files.json", ".atmos-plan-temp", "terraform.tfstate.backup", "other.planfile.atmos.json"} {
		write(ignored)
	}
	same, err := configurationWorkdirDigest(root, plan)
	require.NoError(t, err)
	require.Equal(t, baseline, same, "mutable data does not alter the retained-bytes digest")

	for _, retained := range []string{"modules/child/main.tf", "terragrunt.hcl", ".terraform.lock.hcl"} {
		write(retained)
		changed, err := configurationWorkdirDigest(root, plan)
		require.NoError(t, err)
		require.NotEqual(t, baseline, changed, retained)
		baseline = changed
	}
}

func TestConfigurationWorkdirDigestFailsForUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permissions are not enforced on Windows")
	}
	root := t.TempDir()
	locked := filepath.Join(root, "locked.tf")
	require.NoError(t, os.WriteFile(locked, []byte("secret"), 0o600))
	require.NoError(t, os.Chmod(locked, 0))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o600) })
	if file, err := os.Open(locked); err == nil {
		_ = file.Close()
		t.Skip("permissions are not enforced for this user")
	}
	_, err := configurationWorkdirDigest(root, filepath.Join(root, "reviewed.plan"))
	require.ErrorIs(t, err, os.ErrPermission)
}
