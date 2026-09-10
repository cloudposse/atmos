package cloudformation

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/templates"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPlugin_GetType(t *testing.T) {
	assert.Equal(t, "aws/cloudformation", (&Plugin{}).GetType())
}

func TestPlugin_GetHookBindings(t *testing.T) {
	bindings := (&Plugin{}).GetHookBindings()
	require.Len(t, bindings, 5)

	for _, event := range []string{
		"after.aws/cloudformation.diff",
		"after.aws/cloudformation.apply",
		"after.aws/cloudformation.delete",
		"after.aws/cloudformation.drift-detect",
		"after.aws/cloudformation.drift-describe",
	} {
		t.Run(event, func(t *testing.T) {
			binding := plugin.HookBindings(bindings).GetBindingForEvent(event)
			require.NotNil(t, binding)
			assert.NotNil(t, binding.Handler)
		})
	}
}

func TestPlugin_BuildTemplateContext(t *testing.T) {
	ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
		Command:      "diff",
		Output:       "cloudformation diff output\n",
		CommandError: errors.New("diff failed"),
		ExitCode:     1,
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "tenant1-ue2-dev",
		},
		Aggregate: &schema.CloudFormationCIResult{
			Stack:           "tenant1-ue2-dev",
			Component:       "vpc",
			Command:         "diff",
			StackName:       "tenant1-ue2-dev-vpc",
			ResourceChanges: 3,
		},
	})

	assert.Equal(t, "vpc", ctx.Component)
	assert.Equal(t, "tenant1-ue2-dev", ctx.Stack)
	assert.Equal(t, "diff", ctx.Command)
	assert.Equal(t, "cloudformation diff output", ctx.Output)
	assert.Equal(t, "tenant1-ue2-dev-vpc", ctx.StackName)
	assert.Equal(t, 3, ctx.ResourceChanges)
	require.True(t, ctx.Result.HasErrors)
	assert.Equal(t, []string{"diff failed"}, ctx.Result.Errors)
}

func TestPlugin_BuildTemplateContext_FallsBackToHookContextInfo(t *testing.T) {
	// No Aggregate provided (or the wrong type) — Component/Stack must still
	// come from ctx.Info rather than panicking or leaving them empty.
	ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
		Command: "apply",
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "tenant1-ue2-dev",
		},
	})

	assert.Equal(t, "vpc", ctx.Component)
	assert.Equal(t, "tenant1-ue2-dev", ctx.Stack)
	assert.Empty(t, ctx.StackName)
	assert.False(t, ctx.Result.HasErrors)
}

func TestPlugin_BuildTemplateContext_DriftFields(t *testing.T) {
	ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
		Command: "drift-detect",
		Aggregate: &schema.CloudFormationCIResult{
			StackName:    "tenant1-ue2-dev-vpc",
			DriftStatus:  "DRIFTED",
			DriftedCount: 2,
		},
	})

	assert.Equal(t, "DRIFTED", ctx.DriftStatus)
	assert.Equal(t, 2, ctx.DriftedCount)
}

func TestAggregateResult_HandlesValuePointerAndMissing(t *testing.T) {
	result := aggregateResult(&plugin.HookContext{Aggregate: &schema.CloudFormationCIResult{StackName: "s1"}})
	assert.Equal(t, "s1", result.StackName)

	result = aggregateResult(&plugin.HookContext{Aggregate: schema.CloudFormationCIResult{StackName: "s2"}})
	assert.Equal(t, "s2", result.StackName)

	result = aggregateResult(&plugin.HookContext{Aggregate: nil})
	assert.Equal(t, &schema.CloudFormationCIResult{}, result)

	result = aggregateResult(&plugin.HookContext{Aggregate: "not-a-result"})
	assert.Equal(t, &schema.CloudFormationCIResult{}, result)
}

func TestIsSummaryEnabled(t *testing.T) {
	assert.True(t, isSummaryEnabled(nil))
	assert.True(t, isSummaryEnabled(&schema.AtmosConfiguration{}))

	enabled := true
	assert.True(t, isSummaryEnabled(&schema.AtmosConfiguration{CI: schema.CIConfig{Summary: schema.CISummaryConfig{Enabled: &enabled}}}))

	disabled := false
	assert.False(t, isSummaryEnabled(&schema.AtmosConfiguration{CI: schema.CIConfig{Summary: schema.CISummaryConfig{Enabled: &disabled}}}))
}

func TestTemplateRendering_Diff(t *testing.T) {
	ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
		Command: "diff",
		Output:  "3 resource change(s)",
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "tenant1-ue2-dev",
		},
		Aggregate: &schema.CloudFormationCIResult{
			StackName:       "tenant1-ue2-dev-vpc",
			ResourceChanges: 3,
		},
	})

	rendered, err := templates.NewLoader(nil).LoadAndRender("cloudformation", "diff", defaultTemplates, ctx)
	require.NoError(t, err)
	assert.Contains(t, rendered, "CloudFormation Diff Summary")
	assert.Contains(t, rendered, "atmos aws/cloudformation diff vpc -s tenant1-ue2-dev")
	assert.Contains(t, rendered, "Resource changes: **3**")
}

func TestTemplateRendering_DriftDetectFailed(t *testing.T) {
	ctx := (&Plugin{}).buildTemplateContext(&plugin.HookContext{
		Command:      "drift-detect",
		CommandError: errors.New("drift check failed"),
		ExitCode:     1,
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "tenant1-ue2-dev",
		},
		Aggregate: &schema.CloudFormationCIResult{
			StackName:    "tenant1-ue2-dev-vpc",
			DriftStatus:  "DRIFTED",
			DriftedCount: 2,
		},
	})

	rendered, err := templates.NewLoader(nil).LoadAndRender("cloudformation", "drift-detect", defaultTemplates, ctx)
	require.NoError(t, err)
	assert.Contains(t, rendered, "CloudFormation Drift Detect Failed")
	assert.Contains(t, rendered, "Drift status: **DRIFTED**")
	assert.Contains(t, rendered, "Drifted resources: **2**")
	assert.Contains(t, rendered, "drift check failed")
}
