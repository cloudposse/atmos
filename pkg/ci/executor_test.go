package ci

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	plugin "github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

// stubPlugin is a simple Plugin for tests that uses Handler callbacks.
type stubPlugin struct {
	componentType string
	bindings      []plugin.HookBinding
}

func (s *stubPlugin) GetType() string {
	return s.componentType
}

func (s *stubPlugin) GetHookBindings() []plugin.HookBinding {
	return s.bindings
}

func TestOutputResult(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		result := &plugin.OutputResult{}
		assert.Equal(t, 0, result.ExitCode)
		assert.False(t, result.HasChanges)
		assert.False(t, result.HasErrors)
		assert.Nil(t, result.Errors)
		assert.Nil(t, result.Data)
	})

	t.Run("with terraform data", func(t *testing.T) {
		result := &plugin.OutputResult{
			ExitCode:   0,
			HasChanges: true,
			Data: &plugin.TerraformOutputData{
				ResourceCounts: plugin.ResourceCounts{
					Create:  5,
					Change:  3,
					Destroy: 1,
				},
			},
		}
		assert.True(t, result.HasChanges)
		tfData, ok := result.Data.(*plugin.TerraformOutputData)
		require.True(t, ok)
		assert.Equal(t, 5, tfData.ResourceCounts.Create)
		assert.Equal(t, 3, tfData.ResourceCounts.Change)
		assert.Equal(t, 1, tfData.ResourceCounts.Destroy)
	})
}

func TestResourceCounts(t *testing.T) {
	counts := plugin.ResourceCounts{
		Create:  10,
		Change:  5,
		Replace: 2,
		Destroy: 3,
	}

	assert.Equal(t, 10, counts.Create)
	assert.Equal(t, 5, counts.Change)
	assert.Equal(t, 2, counts.Replace)
	assert.Equal(t, 3, counts.Destroy)
}

func TestTemplateContext(t *testing.T) {
	ctx := &plugin.TemplateContext{
		Component:     "vpc",
		ComponentType: "terraform",
		Stack:         "dev-us-east-1",
		Command:       "plan",
		CI: &provider.Context{
			Provider: "github-actions",
			SHA:      "abc123",
		},
		Result: &plugin.OutputResult{
			HasChanges: true,
		},
		Output: "terraform plan output...",
		Custom: map[string]any{
			"custom_key": "custom_value",
		},
	}

	assert.Equal(t, "vpc", ctx.Component)
	assert.Equal(t, "terraform", ctx.ComponentType)
	assert.Equal(t, "dev-us-east-1", ctx.Stack)
	assert.Equal(t, "plan", ctx.Command)
	assert.NotNil(t, ctx.CI)
	assert.Equal(t, "github-actions", ctx.CI.Provider)
	assert.NotNil(t, ctx.Result)
	assert.True(t, ctx.Result.HasChanges)
	assert.Equal(t, "custom_value", ctx.Custom["custom_key"])
}

func TestMovedResource(t *testing.T) {
	moved := plugin.MovedResource{
		From: "aws_instance.old",
		To:   "aws_instance.new",
	}

	assert.Equal(t, "aws_instance.old", moved.From)
	assert.Equal(t, "aws_instance.new", moved.To)
}

func TestTerraformOutput(t *testing.T) {
	t.Run("string output", func(t *testing.T) {
		output := plugin.TerraformOutput{
			Value:     "vpc-12345",
			Type:      "string",
			Sensitive: false,
		}
		assert.Equal(t, "vpc-12345", output.Value)
		assert.Equal(t, "string", output.Type)
		assert.False(t, output.Sensitive)
	})

	t.Run("sensitive output", func(t *testing.T) {
		output := plugin.TerraformOutput{
			Value:     "secret-password",
			Type:      "string",
			Sensitive: true,
		}
		assert.True(t, output.Sensitive)
	})
}

func TestReleaseInfo(t *testing.T) {
	release := plugin.ReleaseInfo{
		Name:      "my-app",
		Namespace: "production",
		Status:    "deployed",
	}

	assert.Equal(t, "my-app", release.Name)
	assert.Equal(t, "production", release.Namespace)
	assert.Equal(t, "deployed", release.Status)
}

func TestHelmfileOutputData(t *testing.T) {
	data := &plugin.HelmfileOutputData{
		Releases: []plugin.ReleaseInfo{
			{Name: "app1", Namespace: "default", Status: "deployed"},
			{Name: "app2", Namespace: "kube-system", Status: "pending"},
		},
	}

	assert.Len(t, data.Releases, 2)
	assert.Equal(t, "app1", data.Releases[0].Name)
	assert.Equal(t, "app2", data.Releases[1].Name)
}

func TestTerraformOutputData(t *testing.T) {
	data := &plugin.TerraformOutputData{
		ResourceCounts: plugin.ResourceCounts{Create: 5, Change: 3, Destroy: 1},
		CreatedResources: []string{
			"aws_vpc.main",
			"aws_subnet.private[0]",
		},
		UpdatedResources: []string{
			"aws_security_group.web",
		},
		ReplacedResources: []string{
			"aws_instance.web",
		},
		DeletedResources: []string{
			"aws_eip.old",
		},
		MovedResources: []plugin.MovedResource{
			{From: "aws_instance.old", To: "module.compute.aws_instance.main"},
		},
		ImportedResources: []string{
			"aws_s3_bucket.existing",
		},
		Outputs: map[string]plugin.TerraformOutput{
			"vpc_id": {Value: "vpc-123", Type: "string"},
		},
		ChangedResult: "Plan: 5 to add, 3 to change, 1 to destroy.",
	}

	assert.Equal(t, 5, data.ResourceCounts.Create)
	assert.Len(t, data.CreatedResources, 2)
	assert.Len(t, data.UpdatedResources, 1)
	assert.Len(t, data.ReplacedResources, 1)
	assert.Len(t, data.DeletedResources, 1)
	assert.Len(t, data.MovedResources, 1)
	assert.Len(t, data.ImportedResources, 1)
	assert.Len(t, data.Outputs, 1)
	assert.Contains(t, data.ChangedResult, "5 to add")
}
