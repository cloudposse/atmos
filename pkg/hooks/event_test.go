package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHookEvent_IsPostExecution(t *testing.T) {
	tests := []struct {
		name  string
		event HookEvent
		want  bool
	}{
		{name: "before-init is pre-execution", event: BeforeTerraformInit, want: false},
		{name: "after-init is post-execution", event: AfterTerraformInit, want: true},
		{name: "before-plan is pre-execution", event: BeforeTerraformPlan, want: false},
		{name: "after-plan is post-execution", event: AfterTerraformPlan, want: true},
		{name: "after-apply is post-execution", event: AfterTerraformApply, want: true},
		{name: "before-output is pre-execution", event: BeforeTerraformOutput, want: false},
		{name: "after-output is post-execution", event: AfterTerraformOutput, want: true},
		{name: "before-refresh is pre-execution", event: BeforeTerraformRefresh, want: false},
		{name: "after-refresh is post-execution", event: AfterTerraformRefresh, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.event.IsPostExecution())
		})
	}
}

func TestHookEvent_Normalize_InitNotAliased(t *testing.T) {
	// init events have no deploy/apply alias — they normalize to themselves.
	assert.Equal(t, BeforeTerraformInit, BeforeTerraformInit.Normalize())
	assert.Equal(t, AfterTerraformInit, AfterTerraformInit.Normalize())
}

func TestHookEvent_Normalize_OutputRefreshNotAliased(t *testing.T) {
	// output and refresh are read-only operations with no deploy-style alias —
	// they normalize to themselves, and must stay distinct from apply/plan so a
	// hook scoped to one does not cross-fire on the other.
	assert.Equal(t, BeforeTerraformOutput, BeforeTerraformOutput.Normalize())
	assert.Equal(t, AfterTerraformOutput, AfterTerraformOutput.Normalize())
	assert.Equal(t, BeforeTerraformRefresh, BeforeTerraformRefresh.Normalize())
	assert.Equal(t, AfterTerraformRefresh, AfterTerraformRefresh.Normalize())
}

// AWS CloudFormation's CLI verbs (plan, deploy) are aliases for the canonical
// executor-emitted events (diff, apply): the executor only ever fires
// Before/AfterAwsCloudFormationDiff and Before/AfterAwsCloudFormationApply
// (see eventsFor in pkg/component/aws/cloudformation/executor.go), so a hook
// configured for "before.aws/cloudformation.plan" or
// "...deploy" must normalize to the event the executor actually emits, or it
// would never fire.
func TestHookEvent_Normalize_AwsCloudFormationPlanDeployAliased(t *testing.T) {
	assert.Equal(t, BeforeAwsCloudFormationDiff, BeforeAwsCloudFormationPlan.Normalize())
	assert.Equal(t, AfterAwsCloudFormationDiff, AfterAwsCloudFormationPlan.Normalize())
	assert.Equal(t, BeforeAwsCloudFormationApply, BeforeAwsCloudFormationDeploy.Normalize())
	assert.Equal(t, AfterAwsCloudFormationApply, AfterAwsCloudFormationDeploy.Normalize())
}
