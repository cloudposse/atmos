package kubernetes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, "components/kubernetes", config.BasePath)
	assert.Equal(t, ProviderKubectl, config.Provider)
	assert.False(t, config.AutoGenerateFiles)
}

func TestComponentProviderMetadataAndBasePath(t *testing.T) {
	provider := &ComponentProvider{}

	assert.Equal(t, "kubernetes", provider.GetType())
	assert.Equal(t, "Kubernetes", provider.GetGroup())
	assert.Equal(t, "components/kubernetes", provider.GetBasePath(nil))
	assert.Equal(t, "components/kubernetes", provider.GetBasePath(&schema.AtmosConfiguration{}))
	assert.Equal(t, "custom/k8s", provider.GetBasePath(&schema.AtmosConfiguration{
		Components: schema.Components{
			Kubernetes: schema.Kubernetes{BasePath: "custom/k8s"},
		},
	}))
	assert.NoError(t, provider.GenerateArtifacts(&component.ExecutionContext{}))
	assert.Equal(t, []string{"render", "diff", "plan", "apply", "deploy", "delete"}, provider.GetAvailableCommands())
}

func TestComponentProviderListComponents(t *testing.T) {
	provider := &ComponentProvider{}

	components, err := provider.ListComponents(context.Background(), "dev", map[string]any{
		"components": map[string]any{
			"kubernetes": map[string]any{
				"worker": map[string]any{},
				"api":    map[string]any{},
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"api", "worker"}, components)

	components, err = provider.ListComponents(context.Background(), "dev", map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, components)
}

func TestComponentProviderValidateComponent(t *testing.T) {
	provider := &ComponentProvider{}

	assert.NoError(t, provider.ValidateComponent(nil))
	assert.NoError(t, provider.ValidateComponent(map[string]any{
		"metadata": map[string]any{"type": "abstract"},
		"provider": "unknown",
	}))
	assert.NoError(t, provider.ValidateComponent(map[string]any{"provider": ProviderKubectl}))
	assert.NoError(t, provider.ValidateComponent(map[string]any{"provider": ProviderKustomize}))
	require.ErrorContains(t, provider.ValidateComponent(map[string]any{"provider": "helm"}), "provider must be")
}

func TestComponentProviderExecuteDispatchesOperations(t *testing.T) {
	original := executeOperation
	t.Cleanup(func() { executeOperation = original })

	var operations []Operation
	executeOperation = func(ctx *component.ExecutionContext, operation Operation) error {
		operations = append(operations, operation)
		return nil
	}

	provider := &ComponentProvider{}
	for _, subcommand := range []string{"render", "diff", "plan", "apply", "deploy", "delete"} {
		require.NoError(t, provider.Execute(&component.ExecutionContext{SubCommand: subcommand}))
	}

	assert.Equal(t, []Operation{
		OperationRender,
		OperationDiff,
		OperationDiff,
		OperationApply,
		OperationApply,
		OperationDelete,
	}, operations)
}

func TestComponentProviderExecuteRejectsUnsupportedSubcommand(t *testing.T) {
	err := (&ComponentProvider{}).Execute(&component.ExecutionContext{SubCommand: "restart"})
	require.ErrorIs(t, err, errUtils.ErrKubernetesUnsupportedSubcommand)
	require.ErrorContains(t, err, `unsupported kubernetes subcommand: "restart"`)
}
