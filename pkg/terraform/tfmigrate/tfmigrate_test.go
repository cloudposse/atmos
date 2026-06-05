package tfmigrate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{name: "static plan", mode: ModePlan, event: "before.terraform.apply", want: ActionPlan},
		{name: "static apply", mode: ModeApply, event: "before.terraform.plan", want: ActionApply},
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

func TestBackendHistoryEnv_GCS(t *testing.T) {
	env := BackendHistoryEnv("gcs", map[string]any{
		"bucket": "tfstate-bucket",
	})

	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_STORAGE=gcs")
	assert.Contains(t, env, "ATMOS_TFMIGRATE_HISTORY_BUCKET=tfstate-bucket")
}
