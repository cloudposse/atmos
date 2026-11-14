package adapters

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_failure_runtime_test.go -package=$GOPACKAGE -mock_names=failureCheckingRuntime=MockFailureCheckingRuntime

// failureCheckingRuntime is a configuration runtime that reports components it could not evaluate.
type failureCheckingRuntime interface {
	schema.ComponentConfigurationRuntime
	schema.ComponentConfigurationFailureRuntime
}

// The generated mock implements the contract that preflight asserts.
var _ failureCheckingRuntime = (*MockFailureCheckingRuntime)(nil)

// failureRuntime holds the failures the generated runtime reports and records every component it
// is asked about, so tests can assert that each selected component is checked before dispatch.
type failureRuntime struct {
	failures map[string]error
	asked    []string
}

// mock returns a generated runtime that answers ComponentFailure from the fixture. Discover and
// Prepare have no expectations, so gomock rejects them.
func (r *failureRuntime) mock(t *testing.T) *MockFailureCheckingRuntime {
	t.Helper()
	runtime := NewMockFailureCheckingRuntime(gomock.NewController(t))
	runtime.EXPECT().ComponentFailure(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, stack, component string) error {
		r.asked = append(r.asked, stack+"/"+component)
		return r.failures[stack+"/"+component]
	}).AnyTimes()
	return runtime
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
	config := &schema.AtmosConfiguration{ComponentConfigurations: runtime.mock(t)}
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
	require.NoError(t, preflightTerraformGraph(t.Context(), &schema.AtmosConfiguration{ComponentConfigurations: (&failureRuntime{}).mock(t)}, graph))
}

func TestExecuteTerraformPreflightRejectsFailuresBeforeDispatch(t *testing.T) {
	runtime := &failureRuntime{failures: map[string]error{
		"dev/b-broken": errors.New("first component failed validation"),
		"dev/c-broken": errors.New("second component failed validation"),
	}}
	// The healthy component sorts first and has no dependencies, so it would
	// be eligible for dispatch if validation happened only as each node ran.
	stacks := terraformStacksFixture(map[string]any{
		"a-healthy": map[string]any{},
		"b-broken":  map[string]any{},
		"c-broken":  map[string]any{},
	})
	var executorCalls atomic.Int32

	err := ExecuteTerraform(t.Context(), TerraformOptions{
		AtmosConfig: &schema.AtmosConfiguration{ComponentConfigurations: runtime.mock(t)},
		Info:        &schema.ConfigAndStacksInfo{All: true, SubCommand: "plan"},
		Stacks:      stacks,
		Executor: func(TerraformExecution) (TerraformExecutionResult, error) {
			executorCalls.Add(1)
			return TerraformExecutionResult{}, nil
		},
	})

	assert.Zero(t, executorCalls.Load(), "validation must finish before any component is dispatched")
	require.ErrorIs(t, err, errUtils.ErrPluginComponentsFailed)
	assert.ErrorContains(t, err, "first component failed validation")
	assert.ErrorContains(t, err, "second component failed validation")
	assert.ElementsMatch(t, []string{"dev/a-healthy", "dev/b-broken", "dev/c-broken"}, runtime.asked,
		"every selected component must be validated")
}
