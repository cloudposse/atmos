package tfmigrate

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestBuildArgs(t *testing.T) {
	args, err := BuildArgs(Options{
		Action:        ActionApply,
		Migration:     "migrations/001.hcl",
		Config:        ".tfmigrate.hcl",
		BackendConfig: []string{"bucket=state", "key=history"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"apply",
		"--config", ".tfmigrate.hcl",
		"--backend-config=bucket=state",
		"--backend-config=key=history",
		"migrations/001.hcl",
	}, args)
}

func TestBuildArgs_HistoryModeOmitsMigrationFile(t *testing.T) {
	args, err := BuildArgs(Options{Action: ActionPlan})
	require.NoError(t, err)
	assert.Equal(t, []string{"plan"}, args)

	args, err = BuildArgs(Options{Action: ActionApply})
	require.NoError(t, err)
	assert.Equal(t, []string{"apply"}, args)
}

func TestBuildArgs_InvalidAction(t *testing.T) {
	_, err := BuildArgs(Options{Action: "destroy"})
	require.Error(t, err)
}

func TestActionForMode(t *testing.T) {
	tests := []struct {
		name  string
		mode  string
		event string
		want  string
	}{
		{name: "default dynamic plan", event: "before.terraform.plan", want: ActionPlan},
		{name: "dynamic apply", mode: ModeDynamic, event: "before.terraform.apply", want: ActionApply},
		{name: "dynamic deploy", mode: ModeDynamic, event: "before-terraform-deploy", want: ActionApply},
		{name: "static plan on plan event", mode: ModePlan, event: "before.terraform.plan", want: ActionPlan},
		// mode: plan bound to an apply/deploy event is unusual but legitimate
		// (preview-only forever) - it warns, but must not error.
		{name: "static plan on apply event warns but succeeds", mode: ModePlan, event: "before.terraform.apply", want: ActionPlan},
		{name: "static plan on deploy event warns but succeeds", mode: ModePlan, event: "before-terraform-deploy", want: ActionPlan},
		{name: "static apply on apply event", mode: ModeApply, event: "before.terraform.apply", want: ActionApply},
		{name: "static apply on deploy event", mode: ModeApply, event: "before-terraform-deploy", want: ActionApply},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ActionForMode(tt.mode, tt.event)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestActionForMode_RejectsUnsupportedDynamicEvent(t *testing.T) {
	_, err := ActionForMode(ModeDynamic, "after.terraform.plan")
	require.Error(t, err)
}

func TestActionForMode_RejectsModeApplyOnNonApplyEvent(t *testing.T) {
	// mode: apply bound to a plan (or any other non-apply/deploy) event would
	// otherwise silently mutate real state on what looks like a read-only
	// operation - this must be a hard error, unlike the mode: plan direction.
	tests := []struct {
		name  string
		event string
	}{
		{name: "plan event", event: "before.terraform.plan"},
		{name: "unrelated after-event", event: "after.terraform.apply"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ActionForMode(ModeApply, tt.event)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrInvalidConfig)
		})
	}
}

func TestAppendExecPath(t *testing.T) {
	env := AppendExecPath([]string{"A=B"}, "/bin/tofu")
	assert.Contains(t, env, "TFMIGRATE_EXEC_PATH=/bin/tofu")

	env = AppendExecPath([]string{"TFMIGRATE_EXEC_PATH=/custom/terraform"}, "/bin/tofu")
	assert.Equal(t, []string{"TFMIGRATE_EXEC_PATH=/custom/terraform"}, env)
}

func TestHistoryEnv(t *testing.T) {
	env := HistoryEnv("plat-ue2-dev", "s3-bucket", "")

	assert.Contains(t, env, "ATMOS_STACK=plat-ue2-dev")
	assert.Contains(t, env, "ATMOS_COMPONENT=s3-bucket")
	assert.Contains(t, env, "ATMOS_TERRAFORM_WORKSPACE=default")
	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_NAMESPACE=tfmigrate/plat-ue2-dev/s3-bucket/default")
	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_KEY=tfmigrate/plat-ue2-dev/s3-bucket/default/history.json")
	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_PATH=tfmigrate/plat-ue2-dev/s3-bucket/default/history.json")
}

func TestHistoryEnv_UsesWorkspace(t *testing.T) {
	env := HistoryEnv("plat-ue2-dev", "s3-bucket", "prod")

	assert.Contains(t, env, "ATMOS_TERRAFORM_WORKSPACE=prod")
	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_KEY=tfmigrate/plat-ue2-dev/s3-bucket/prod/history.json")
}

func TestBackendHistoryEnv_S3(t *testing.T) {
	env := BackendHistoryEnv("s3", map[string]any{
		"bucket":  "tfstate-bucket",
		"region":  "us-east-1",
		"profile": "dev",
		"assume_role": map[string]any{
			"role_arn": "arn:aws:iam::123456789012:role/tfstate",
		},
		"endpoint":   "http://localhost:4566",
		"kms_key_id": "alias/tfstate",
	})

	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_STORAGE=s3")
	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_BUCKET=tfstate-bucket")
	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_REGION=us-east-1")
	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_PROFILE=dev")
	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_ROLE_ARN=arn:aws:iam::123456789012:role/tfstate")
	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_ENDPOINT=http://localhost:4566")
	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_KMS_KEY_ID=alias/tfstate")
}

func TestBackendHistoryEnv_S3NestedEndpoints(t *testing.T) {
	// Terraform 1.6+ moved the S3 endpoint under `endpoints { s3 = ... }`.
	env := BackendHistoryEnv("s3", map[string]any{
		"bucket": "tfstate-bucket",
		"endpoints": map[string]any{
			"s3": "http://localhost:4566",
		},
	})

	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_ENDPOINT=http://localhost:4566")
}

func TestBackendHistoryEnv_S3LegacyEndpointWins(t *testing.T) {
	env := BackendHistoryEnv("s3", map[string]any{
		"bucket":   "tfstate-bucket",
		"endpoint": "http://legacy:4566",
		"endpoints": map[string]any{
			"s3": "http://nested:4566",
		},
	})

	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_ENDPOINT=http://legacy:4566")
}

func TestBackendHistoryEnv_GCS(t *testing.T) {
	env := BackendHistoryEnv("gcs", map[string]any{
		"bucket": "tfstate-bucket",
	})

	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_STORAGE=gcs")
	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_BUCKET=tfstate-bucket")
}

func TestOptionsValidate_InvalidAction(t *testing.T) {
	err := Options{Action: "destroy"}.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidConfig)
}

func TestBuildArgs_SkipsEmptyBackendConfigEntries(t *testing.T) {
	args, err := BuildArgs(Options{
		Action:        ActionPlan,
		BackendConfig: []string{"", "bucket=state", ""},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"plan", "--backend-config=bucket=state"}, args)
}

func TestEnsureResolved_FindsCommandOnPATH(t *testing.T) {
	// Point PATH at a temp dir containing an executable literally named
	// "tfmigrate" so exec.LookPath(Command) succeeds, exercising the
	// found-on-PATH branch without requiring the real tool to be installed.
	dir := t.TempDir()
	name := Command
	if runtime.GOOS == "windows" {
		name += ".bat"
	}
	fakeBin := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("PATH", dir)

	err := EnsureResolved(Command)
	assert.NoError(t, err)
}

func TestActionForMode_RejectsUnsupportedMode(t *testing.T) {
	_, err := ActionForMode("bogus-mode", "before.terraform.plan")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidConfig)
}

func TestAppendExecPath_RespectsProcessEnvVar(t *testing.T) {
	// hasEnvKey only inspects the env slice being built; when TFMIGRATE_EXEC_PATH
	// is instead set in the process environment, AppendExecPath must still defer
	// to it rather than appending a duplicate.
	t.Setenv(ExecPathEnvVar, "/from/process/env")

	env := AppendExecPath([]string{"A=B"}, "/bin/tofu")
	assert.Equal(t, []string{"A=B"}, env)
}

func TestBackendHistoryValues_EmptyBackendReturnsNil(t *testing.T) {
	assert.Nil(t, BackendHistoryValues("s3", nil))
	assert.Nil(t, BackendHistoryValues("s3", map[string]any{}))
}

// TestBackendHistoryValues_UnsupportedBackendTypeReportsStorageOnly covers
// backend types other than s3/gcs (which fall back to local-workdir tfmigrate
// history storage, see default_config.go's writeLocalHistoryStorage): they
// still report EnvHistoryStorage = the real backend type (so `migrate list`'s
// History Storage column isn't misleadingly blank for the most common,
// zero-config case, "local", or any other backend), just with no bucket/
// region/etc. extras, since only s3/gcs have those to surface.
func TestBackendHistoryValues_UnsupportedBackendTypeReportsStorageOnly(t *testing.T) {
	values := BackendHistoryValues("azurerm", map[string]any{"storage_account_name": "acct"})
	require.NotNil(t, values)
	assert.Equal(t, "azurerm", values[EnvHistoryStorage])
	assert.NotContains(t, values, EnvHistoryBucket)

	values = BackendHistoryValues("local", map[string]any{"path": "state.tfstate"})
	require.NotNil(t, values)
	assert.Equal(t, "local", values[EnvHistoryStorage])
}

func TestBackendHistoryValues_GCSNestedBackendBlock(t *testing.T) {
	// Mirrors how Atmos component backend sections nest provider-specific config
	// under the backend type key, e.g. `backend: { gcs: { bucket: ... } }`.
	values := BackendHistoryValues("gcs", map[string]any{
		"gcs": map[string]any{"bucket": "nested-bucket"},
	})
	assert.Equal(t, "nested-bucket", values[EnvHistoryBucket])
}

func TestHistoryPath_AllEmptyPartsReturnsEmptyString(t *testing.T) {
	assert.Equal(t, "", historyPath("", "", ""))
	assert.Equal(t, "", historyPath())
}
