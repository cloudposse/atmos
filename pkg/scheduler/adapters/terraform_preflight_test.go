package adapters

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
)

type failureRuntime struct {
	schema.ComponentConfigurationRuntime
	failures map[string]error
	asked    []string
}

func (r *failureRuntime) ComponentFailure(_ context.Context, stack, component string) error {
	r.asked = append(r.asked, stack+"/"+component)
	return r.failures[stack+"/"+component]
}

func graphOf(t *testing.T, ids ...[2]string) *dependency.Graph {
	t.Helper()
	builder := dependency.NewBuilder()
	for _, id := range ids {
		require.NoError(t, builder.AddNode(&dependency.Node{ID: terraformNodeID(id[1], id[0]), Stack: id[0], Component: id[1]}))
	}
	graph, err := builder.Build()
	require.NoError(t, err)
	return graph
}

func TestPreflightRejectsEveryBrokenComponentBeforeAnyRuns(t *testing.T) {
	runtime := &failureRuntime{failures: map[string]error{"live/a": errors.New("first"), "live/c": errors.New("second")}}
	config := &schema.AtmosConfiguration{ComponentConfigurations: runtime}
	graph := graphOf(t, [2]string{"live", "a"}, [2]string{"live", "b"}, [2]string{"live", "c"})

	err := preflightTerraformGraph(t.Context(), config, graph)
	require.ErrorIs(t, err, errUtils.ErrPluginComponentsFailed)
	assert.ErrorContains(t, err, "first")
	assert.ErrorContains(t, err, "second")
	assert.Len(t, runtime.asked, 3, "every selected component is checked")
}

func TestPreflightIgnoresHealthyGraphsAndProvidersWithoutFailures(t *testing.T) {
	graph := graphOf(t, [2]string{"live", "a"})
	require.NoError(t, preflightTerraformGraph(t.Context(), &schema.AtmosConfiguration{}, graph))
	require.NoError(t, preflightTerraformGraph(t.Context(), &schema.AtmosConfiguration{ComponentConfigurations: &failureRuntime{}}, graph))
}
