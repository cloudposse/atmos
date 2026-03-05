package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	// Should have 4 bindings: before.plan, after.plan, before.apply, after.apply.
	require.Len(t, bindings, 4)

	// Verify all bindings have handlers.
	expectedEvents := []string{
		"before.terraform.plan",
		"after.terraform.plan",
		"before.terraform.apply",
		"after.terraform.apply",
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

	result, err := p.buildTemplateContext(info, ciCtx, output, "plan")
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

	result, err := p.buildTemplateContext(info, nil, output, "plan")
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

	result, err := p.buildTemplateContext(info, nil, output, "plan")
	require.NoError(t, err)

	ctx, ok := result.(*TerraformTemplateContext)
	require.True(t, ok)

	assert.Empty(t, ctx.Output, "Output should be empty for no-changes plan")
}

func TestPlugin_BuildTemplateContext_PreservesOutputWithoutMarkers(t *testing.T) {
	p := &Plugin{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "dev-us-east-1",
	}

	// Output without any known markers should be preserved as-is.
	output := "Some unknown terraform output"

	result, err := p.buildTemplateContext(info, nil, output, "plan")
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

	t.Run("valid stack and component", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "dev-us-east-1",
		}
		key := p.getArtifactKey(info, "plan")
		assert.Equal(t, "dev-us-east-1/vpc.tfplan", key)
	})

	t.Run("nil info returns placeholder", func(t *testing.T) {
		key := p.getArtifactKey(nil, "plan")
		assert.Equal(t, "unknown/unknown.tfplan", key)
	})

	t.Run("empty stack uses placeholder", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "",
		}
		key := p.getArtifactKey(info, "plan")
		assert.Equal(t, "unknown/vpc.tfplan", key)
	})

	t.Run("empty component uses placeholder", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "",
			Stack:            "dev-us-east-1",
		}
		key := p.getArtifactKey(info, "plan")
		assert.Equal(t, "dev-us-east-1/unknown.tfplan", key)
	})

	t.Run("both empty uses placeholders", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "",
			Stack:            "",
		}
		key := p.getArtifactKey(info, "plan")
		assert.Equal(t, "unknown/unknown.tfplan", key)
	})
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
