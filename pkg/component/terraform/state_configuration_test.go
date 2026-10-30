package terraform

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	componentpkg "github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

// failingStateRuntime owns the component but cannot identify its backend.
type failingStateRuntime struct {
	schema.ComponentConfigurationRuntime
	err error
}

func (r failingStateRuntime) ResolveState(context.Context, *schema.ConfigAndStacksInfo) (*schema.ComponentConfigurationState, error) {
	return nil, r.err
}

func TestGetTerraformStateStopsWhenProviderCannotResolveBackend(t *testing.T) {
	failure := errors.New("provider could not locate state")
	executor := testExecutor(t)
	executor.services.ExecuteDescribeComponent = func(params *componentpkg.ExecuteDescribeComponentParams) (map[string]any, error) {
		assert.Equal(t, "network", params.Component)
		assert.Equal(t, "live", params.Stack)
		return map[string]any{
			"settings": map[string]any{"configuration": true},
			"backend":  map[string]any{"path": "must-not-be-read"},
		}, nil
	}
	config := &schema.AtmosConfiguration{ComponentConfigurations: failingStateRuntime{err: failure}}

	result, err := executor.GetTerraformState(config, "!terraform.state", "live", "network", "vpc_id", true, nil, nil)
	require.ErrorIs(t, err, failure, "the provider's failure is returned as is, not retried against a guessed backend")
	require.Nil(t, result)
}
