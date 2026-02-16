package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPlugin_GetType(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "terraform", p.GetType())
}

func TestPlugin_GetHookBindings(t *testing.T) {
	p := &Plugin{}
	bindings := p.GetHookBindings()

	// Should have bindings for plan, apply, and before.apply (download).
	require.Len(t, bindings, 3)

	// Check after.terraform.plan binding.
	planBinding := findBinding(bindings, "after.terraform.plan")
	require.NotNil(t, planBinding)
	assert.Equal(t, "plan", planBinding.Template)
	assert.True(t, planBinding.HasAction(ci.ActionSummary))
	assert.True(t, planBinding.HasAction(ci.ActionOutput))
	assert.True(t, planBinding.HasAction(ci.ActionUpload))

	// Check after.terraform.apply binding.
	applyBinding := findBinding(bindings, "after.terraform.apply")
	require.NotNil(t, applyBinding)
	assert.Equal(t, "apply", applyBinding.Template)
	assert.True(t, applyBinding.HasAction(ci.ActionSummary))
	assert.True(t, applyBinding.HasAction(ci.ActionOutput))
	assert.False(t, applyBinding.HasAction(ci.ActionUpload))

	// Check before.terraform.apply binding (download).
	downloadBinding := findBinding(bindings, "before.terraform.apply")
	require.NotNil(t, downloadBinding)
	assert.Empty(t, downloadBinding.Template)
	assert.True(t, downloadBinding.HasAction(ci.ActionDownload))
}

func TestPlugin_GetDefaultTemplates(t *testing.T) {
	p := &Plugin{}
	fs := p.GetDefaultTemplates()

	// Should be able to read plan.md.
	content, err := fs.ReadFile("templates/plan.md")
	require.NoError(t, err)
	assert.Contains(t, string(content), "terraform plan")
	assert.Contains(t, string(content), ".Resources.Create")

	// Should be able to read apply.md.
	content, err = fs.ReadFile("templates/apply.md")
	require.NoError(t, err)
	assert.Contains(t, string(content), "terraform apply")
	assert.Contains(t, string(content), ".Outputs")
}

func TestPlugin_BuildTemplateContext(t *testing.T) {
	p := &Plugin{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "dev-us-east-1",
	}
	ciCtx := &ci.Context{
		SHA:        "abc123",
		Repository: "owner/repo",
		Actor:      "testuser",
	}
	output := "Plan: 1 to add, 0 to change, 0 to destroy."

	result, err := p.BuildTemplateContext(info, ciCtx, output, "plan")
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

func TestPlugin_ParseOutput(t *testing.T) {
	p := &Plugin{}

	result, err := p.ParseOutput("Plan: 5 to add, 2 to change, 1 to destroy.", "plan")
	require.NoError(t, err)
	assert.True(t, result.HasChanges)

	data, ok := result.Data.(*ci.TerraformOutputData)
	require.True(t, ok)
	assert.Equal(t, 5, data.ResourceCounts.Create)
	assert.Equal(t, 2, data.ResourceCounts.Change)
	assert.Equal(t, 1, data.ResourceCounts.Destroy)
}

func TestPlugin_GetOutputVariables(t *testing.T) {
	p := &Plugin{}
	result := &ci.OutputResult{
		ExitCode:   0,
		HasChanges: true,
		HasErrors:  false,
		Data: &ci.TerraformOutputData{
			ResourceCounts: ci.ResourceCounts{
				Create:  3,
				Change:  2,
				Replace: 1,
				Destroy: 0,
			},
		},
	}

	vars := p.GetOutputVariables(result, "plan")

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
		key := p.GetArtifactKey(info, "plan")
		assert.Equal(t, "dev-us-east-1/vpc.tfplan", key)
	})

	t.Run("nil info returns placeholder", func(t *testing.T) {
		key := p.GetArtifactKey(nil, "plan")
		assert.Equal(t, "unknown/unknown.tfplan", key)
	})

	t.Run("empty stack uses placeholder", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "",
		}
		key := p.GetArtifactKey(info, "plan")
		assert.Equal(t, "unknown/vpc.tfplan", key)
	})

	t.Run("empty component uses placeholder", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "",
			Stack:            "dev-us-east-1",
		}
		key := p.GetArtifactKey(info, "plan")
		assert.Equal(t, "dev-us-east-1/unknown.tfplan", key)
	})

	t.Run("both empty uses placeholders", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: "",
			Stack:            "",
		}
		key := p.GetArtifactKey(info, "plan")
		assert.Equal(t, "unknown/unknown.tfplan", key)
	})
}

// Helper function to find a binding by event.
func findBinding(bindings []ci.HookBinding, event string) *ci.HookBinding {
	for i := range bindings {
		if bindings[i].Event == event {
			return &bindings[i]
		}
	}
	return nil
}
