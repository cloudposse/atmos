package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPlugin_GetType(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "terraform", p.GetType())
}

func TestPlugin_GetHookBindings(t *testing.T) {
	p := &Plugin{}
	bindings := p.GetHookBindings()

	// Should have 6 bindings: before/after for plan, apply, deploy.
	require.Len(t, bindings, 6)

	// Verify all bindings have handlers.
	expectedEvents := []string{
		"before.terraform.plan",
		"after.terraform.plan",
		"before.terraform.apply",
		"after.terraform.apply",
		"before.terraform.deploy",
		"after.terraform.deploy",
	}

	for _, expectedEvent := range expectedEvents {
		binding := findBinding(bindings, expectedEvent)
		require.NotNil(t, binding, "binding for %s should exist", expectedEvent)
		assert.NotNil(t, binding.Handler, "binding for %s should have a handler", expectedEvent)
	}
}

func TestPlugin_BuildTemplateContext(t *testing.T) {
	p := &Plugin{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "dev-us-east-1",
	}
	ciCtx := &provider.Context{
		SHA:        "abc123",
		Repository: "owner/repo",
		Actor:      "testuser",
	}
	output := "Plan: 1 to add, 0 to change, 0 to destroy."

	result, err := p.buildTemplateContext(info, ciCtx, output, "plan", nil)
	require.NoError(t, err)

	// Should return TerraformTemplateContext.
	ctx, ok := result.(*TerraformTemplateContext)
	require.True(t, ok, "Expected *TerraformTemplateContext")

	assert.Equal(t, "vpc", ctx.Component)
	assert.Equal(t, "terraform", ctx.ComponentType)
	assert.Equal(t, "dev-us-east-1", ctx.Stack)
	assert.Equal(t, "plan", ctx.Command)
	assert.Equal(t, ciCtx, ctx.CI)
	assert.Equal(t, output, ctx.Output)
	assert.NotNil(t, ctx.Result)

	// Check terraform-specific fields.
	assert.Equal(t, 1, ctx.Resources.Create)
	assert.True(t, ctx.HasChanges())
}

func TestPlugin_BuildTemplateContext_StripsOutputBeforePlanActions(t *testing.T) {
	p := &Plugin{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "dev-us-east-1",
	}

	// Simulate realistic terraform plan output with noise before the plan actions.
	output := `data.validation_warning.warn[0]: Reading...
data.validation_warning.warn[0]: Read complete after 0s [id=none]

Terraform used the selected providers to generate the following execution
plan. Resource actions are indicated with the following symbols:
  + create

Terraform will perform the following actions:

  # null_resource.test will be created
  + resource "null_resource" "test" {
      + id = (known after apply)
    }

Plan: 1 to add, 0 to change, 0 to destroy.`

	result, err := p.buildTemplateContext(info, nil, output, "plan", nil)
	require.NoError(t, err)

	ctx, ok := result.(*TerraformTemplateContext)
	require.True(t, ok)

	// Output should NOT contain the data source reading noise.
	assert.NotContains(t, ctx.Output, "data.validation_warning.warn")
	assert.NotContains(t, ctx.Output, "Reading...")
	assert.NotContains(t, ctx.Output, "Read complete after")

	// Output SHOULD start from after "Terraform will perform the following actions:".
	assert.NotContains(t, ctx.Output, "Terraform will perform the following actions:")
	assert.Contains(t, ctx.Output, "null_resource.test")
	assert.Contains(t, ctx.Output, "Plan: 1 to add, 0 to change, 0 to destroy.")
}

func TestPlugin_BuildTemplateContext_ClearsOutputForNoChanges(t *testing.T) {
	p := &Plugin{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "dev-us-east-1",
	}

	// No-changes output has no plan to display in the summary section.
	output := "No changes. Your infrastructure matches the configuration."

	result, err := p.buildTemplateContext(info, nil, output, "plan", nil)
	require.NoError(t, err)

	ctx, ok := result.(*TerraformTemplateContext)
	require.True(t, ok)

	assert.Empty(t, ctx.Output, "Output should be empty for no-changes plan")
}

func TestPlugin_BuildTemplateContext_ApplyKeepsOutputWithNoChanges(t *testing.T) {
	p := &Plugin{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "dev-us-east-1",
	}

	// Apply with no changes should still show the apply result.
	output := "No changes. Your infrastructure matches the configuration.\n\nApply complete! Resources: 0 added, 0 changed, 0 destroyed."

	result, err := p.buildTemplateContext(info, nil, output, "apply", nil)
	require.NoError(t, err)

	ctx, ok := result.(*TerraformTemplateContext)
	require.True(t, ok)

	assert.Contains(t, ctx.Output, "Apply complete!")
	assert.NotEmpty(t, ctx.Output, "Apply output should not be empty even with no changes")
}

func TestPlugin_BuildTemplateContext_ApplyStripsProgressLines(t *testing.T) {
	p := &Plugin{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "dev-us-east-1",
	}

	// Apply output with progress lines before the result.
	output := "aws_instance.web: Creating...\naws_instance.web: Creation complete after 35s [id=i-12345678]\n\nApply complete! Resources: 1 added, 0 changed, 0 destroyed.\n\nOutputs:\n\ninstance_id = \"i-12345678\""

	result, err := p.buildTemplateContext(info, nil, output, "apply", nil)
	require.NoError(t, err)

	ctx, ok := result.(*TerraformTemplateContext)
	require.True(t, ok)

	assert.True(t, len(ctx.Output) > 0)
	assert.Contains(t, ctx.Output, "Apply complete!")
	assert.NotContains(t, ctx.Output, "Creating...")
	assert.NotContains(t, ctx.Output, "Creation complete")
	assert.Contains(t, ctx.Output, "instance_id")
}

func TestPlugin_BuildTemplateContext_ApplyKeepsPlanDiffs(t *testing.T) {
	p := &Plugin{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "app",
		Stack:            "dev-us-east-1",
	}

	// Realistic apply output with plan diffs, progress lines, and result.
	output := `data.validation_warning.warn[0]: Reading...
data.validation_warning.warn[0]: Read complete after 0s [id=none]

OpenTofu used the selected providers to generate the following execution
plan. Resource actions are indicated with the following symbols:
  ~ update in-place

OpenTofu will perform the following actions:

  # aws_ecs_service.default will be updated in-place
  ~ resource "aws_ecs_service" "default" {
      ~ task_definition = "arn:aws:ecs:us-east-2:123:task-definition/app:8" -> (known after apply)
    }

Plan: 1 to add, 1 to change, 0 to destroy.
aws_ecs_task_definition.default: Creating...
aws_ecs_task_definition.default: Creation complete after 0s [id=app]
aws_ecs_service.default: Modifying... [id=arn:aws:ecs:us-east-2:123:service/app]
aws_ecs_service.default: Still modifying... [id=arn:aws:ecs:..., 10s elapsed]
aws_ecs_service.default: Modifications complete after 30s [id=arn:aws:ecs:us-east-2:123:service/app]

Apply complete! Resources: 1 added, 1 changed, 0 destroyed.

Outputs:

url = "http://app.example.com"`

	result, err := p.buildTemplateContext(info, nil, output, "apply", nil)
	require.NoError(t, err)

	ctx, ok := result.(*TerraformTemplateContext)
	require.True(t, ok)

	// Should contain plan diffs.
	assert.Contains(t, ctx.Output, "aws_ecs_service.default will be updated in-place")
	assert.Contains(t, ctx.Output, "task_definition")
	assert.Contains(t, ctx.Output, "Plan: 1 to add, 1 to change, 0 to destroy.")

	// Should contain apply result and outputs.
	assert.Contains(t, ctx.Output, "Apply complete!")
	assert.Contains(t, ctx.Output, "url")

	// Should NOT contain pre-plan noise.
	assert.NotContains(t, ctx.Output, "data.validation_warning.warn")
	assert.NotContains(t, ctx.Output, "OpenTofu will perform the following actions:")

	// Should NOT contain apply progress lines.
	assert.NotContains(t, ctx.Output, "Creating...")
	assert.NotContains(t, ctx.Output, "Creation complete")
	assert.NotContains(t, ctx.Output, "Modifying...")
	assert.NotContains(t, ctx.Output, "Still modifying...")
	assert.NotContains(t, ctx.Output, "Modifications complete")
}

func TestPlugin_BuildTemplateContext_PreservesOutputWithoutMarkers(t *testing.T) {
	p := &Plugin{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "dev-us-east-1",
	}

	// Output without any known markers should be preserved as-is.
	output := "Some unknown terraform output"

	result, err := p.buildTemplateContext(info, nil, output, "plan", nil)
	require.NoError(t, err)

	ctx, ok := result.(*TerraformTemplateContext)
	require.True(t, ok)

	assert.Equal(t, output, ctx.Output)
}

func TestPlugin_ParseOutput(t *testing.T) {
	// ParseOutput is now a package-level function.
	result := ParseOutput("Plan: 5 to add, 2 to change, 1 to destroy.", "plan")
	assert.True(t, result.HasChanges)

	data, ok := result.Data.(*plugin.TerraformOutputData)
	require.True(t, ok)
	assert.Equal(t, 5, data.ResourceCounts.Create)
	assert.Equal(t, 2, data.ResourceCounts.Change)
	assert.Equal(t, 1, data.ResourceCounts.Destroy)
}

func TestPlugin_GetOutputVariables(t *testing.T) {
	p := &Plugin{}
	result := &plugin.OutputResult{
		ExitCode:   0,
		HasChanges: true,
		HasErrors:  false,
		Data: &plugin.TerraformOutputData{
			ResourceCounts: plugin.ResourceCounts{
				Create:  3,
				Change:  2,
				Replace: 1,
				Destroy: 0,
			},
		},
	}

	vars := p.getOutputVariables(result, "plan")

	assert.Equal(t, "true", vars["has_changes"])
	assert.Equal(t, "false", vars["has_errors"])
	assert.Equal(t, "0", vars["exit_code"])
	assert.Equal(t, "3", vars["resources_to_create"])
	assert.Equal(t, "2", vars["resources_to_change"])
	assert.Equal(t, "1", vars["resources_to_replace"])
	assert.Equal(t, "0", vars["resources_to_destroy"])
}

func TestPlugin_GetArtifactKey(t *testing.T) {
	p := &Plugin{}

	t.Run("valid stack component and SHA", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "dev-us-east-1",
		}
		ciCtx := &provider.Context{SHA: "abc123"}
		key, err := p.getArtifactKey(info, ciCtx)
		require.NoError(t, err)
		assert.Equal(t, "dev-us-east-1/vpc/abc123.tfplan.tar", key)
	})

	t.Run("nil info returns error", func(t *testing.T) {
		ciCtx := &provider.Context{SHA: "abc123"}
		_, err := p.getArtifactKey(nil, ciCtx)
		assert.Error(t, err)
	})

	t.Run("empty stack returns error", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "",
		}
		ciCtx := &provider.Context{SHA: "abc123"}
		_, err := p.getArtifactKey(info, ciCtx)
		assert.ErrorIs(t, err, errUtils.ErrPlanfileKeyInvalid)
	})

	t.Run("empty component returns error", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "",
			Stack:            "dev-us-east-1",
		}
		ciCtx := &provider.Context{SHA: "abc123"}
		_, err := p.getArtifactKey(info, ciCtx)
		assert.ErrorIs(t, err, errUtils.ErrPlanfileKeyInvalid)
	})

	t.Run("empty SHA returns error", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "dev-us-east-1",
		}
		ciCtx := &provider.Context{SHA: ""}
		_, err := p.getArtifactKey(info, ciCtx)
		assert.ErrorIs(t, err, errUtils.ErrPlanfileKeyInvalid)
	})

	t.Run("nil CI context returns error", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "dev-us-east-1",
		}
		_, err := p.getArtifactKey(info, nil)
		assert.ErrorIs(t, err, errUtils.ErrPlanfileKeyInvalid)
	})
}

// TestBuildTemplateContext_PreservesPassedResult ensures that when a caller
// supplies an enriched *plugin.OutputResult (e.g., the result from
// parseOutputWithError that has HasErrors=true because ctx.CommandError != nil),
// buildTemplateContext propagates it into the template context instead of
// re-parsing the raw output and dropping the error context.
func TestBuildTemplateContext_PreservesPassedResult(t *testing.T) {
	p := &Plugin{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "dev",
	}

	enriched := &plugin.OutputResult{
		HasErrors: true,
		ExitCode:  1,
		Errors:    []string{"boom"},
		Data:      &plugin.TerraformOutputData{},
	}

	result, err := p.buildTemplateContext(info, nil, "", "apply", enriched)
	require.NoError(t, err)

	ctx, ok := result.(*TerraformTemplateContext)
	require.True(t, ok, "Expected *TerraformTemplateContext")
	require.NotNil(t, ctx.Result)

	assert.True(t, ctx.Result.HasErrors, "HasErrors must reflect the passed-in enriched result")
	require.Len(t, ctx.Result.Errors, 1)
	assert.Equal(t, "boom", ctx.Result.Errors[0])
	assert.Equal(t, 1, ctx.Result.ExitCode)
}

// TestBuildTemplateContext_FallsBackToParseWhenNil verifies that legacy callers
// passing nil for the result argument still get the old behavior (re-parse the
// output). This guards backward compatibility for any future callers and makes
// the fallback contract explicit.
func TestBuildTemplateContext_FallsBackToParseWhenNil(t *testing.T) {
	p := &Plugin{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "dev",
	}

	result, err := p.buildTemplateContext(info, nil, "Plan: 2 to add, 0 to change, 0 to destroy.", "plan", nil)
	require.NoError(t, err)

	ctx, ok := result.(*TerraformTemplateContext)
	require.True(t, ok)
	require.NotNil(t, ctx.Result)

	assert.False(t, ctx.Result.HasErrors)
	assert.Equal(t, 2, ctx.Resources.Create)
	assert.True(t, ctx.HasChanges())
}

// Helper function to find a binding by event.
func findBinding(bindings []plugin.HookBinding, event string) *plugin.HookBinding {
	for i := range bindings {
		if bindings[i].Event == event {
			return &bindings[i]
		}
	}
	return nil
}
