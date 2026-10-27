package terraform

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProviderInventory(t *testing.T) {
	p := New(nil)
	assert.Equal(t, "terraform", p.GetType())
	assert.Equal(t, "Infrastructure as Code", p.GetGroup())
	assert.Equal(t, "components/terraform", p.GetBasePath(nil))
	config := &schema.AtmosConfiguration{}
	assert.Equal(t, "components/terraform", p.GetBasePath(config))
	config.Components.Terraform.BasePath = "infra/modules"
	assert.Equal(t, "infra/modules", p.GetBasePath(config))
	commands := p.GetAvailableCommands()
	assert.Contains(t, commands, "plan")
	assert.Contains(t, commands, "workspace")
	commands[0] = "modified"
	assert.Equal(t, "apply", p.GetAvailableCommands()[0])
	for _, stack := range []map[string]any{nil, {}, {"components": "invalid"}, {"components": map[string]any{"helm": map[string]any{"release": nil}}}} {
		names, err := p.ListComponents(t.Context(), "dev", stack)
		require.NoError(t, err)
		assert.Empty(t, names)
	}
	names, err := p.ListComponents(t.Context(), "dev", map[string]any{"components": map[string]any{"terraform": map[string]any{"network": nil, "app": nil}}})
	require.NoError(t, err)
	assert.Equal(t, []string{"app", "network"}, names)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = p.ListComponents(ctx, "dev", nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestValidateBackendType(t *testing.T) {
	p := New(nil)
	for _, config := range []map[string]any{nil, {}, {"backend_type": nil}, {"backend_type": "s3"}} {
		require.NoError(t, p.ValidateComponent(config))
	}
	require.ErrorIs(t, p.ValidateComponent(map[string]any{"backend_type": 42}), errUtils.ErrComponentConfigInvalid)
}

func TestProviderExecution(t *testing.T) {
	runtime := NewMockRuntime(gomock.NewController(t))
	p := New(runtime)
	request := &component.ExecutionContext{Context: t.Context(), Component: "app", Stack: "dev", SubCommand: "plan"}
	// Detailed-exit-code and cancellation sentinels must retain their identity.
	for _, result := range []error{nil, errUtils.ErrPlanHasDiff, context.Canceled} {
		runtime.EXPECT().Execute(gomock.Eq(request)).Return(result)
		require.ErrorIs(t, p.Execute(request), result)
	}
	request.AtmosConfig, request.WorkingDir = &schema.AtmosConfiguration{}, t.TempDir()
	runtime.EXPECT().GenerateArtifacts(gomock.Eq(request)).Return(nil)
	require.NoError(t, p.GenerateArtifacts(request))
	runtime.EXPECT().GenerateArtifacts(gomock.Eq(request)).Return(errUtils.ErrCreateDirectory)
	require.ErrorIs(t, p.GenerateArtifacts(request), errUtils.ErrCreateDirectory)
}

func TestProviderRejectsIncompleteRequests(t *testing.T) {
	runtime := NewMockRuntime(gomock.NewController(t)) // No calls are allowed on rejected requests.
	p := New(runtime)
	for _, operation := range []func(*component.ExecutionContext) error{p.Execute, p.GenerateArtifacts} {
		require.ErrorIs(t, operation(nil), errUtils.ErrComponentConfigInvalid)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, operation(&component.ExecutionContext{Context: ctx}), context.Canceled)
	}
	require.ErrorIs(t, New(nil).Execute(&component.ExecutionContext{}), errUtils.ErrComponentExecutionFailed)
	require.ErrorIs(t, p.GenerateArtifacts(&component.ExecutionContext{}), errUtils.ErrComponentConfigInvalid)
	require.ErrorIs(t, p.GenerateArtifacts(&component.ExecutionContext{AtmosConfig: &schema.AtmosConfiguration{}}), errUtils.ErrComponentConfigInvalid)
}
