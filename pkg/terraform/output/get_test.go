package output

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSetDefaultExecutor(t *testing.T) {
	// Save original executor.
	originalExecutor := defaultExecutor
	defer func() {
		defaultExecutor = originalExecutor
	}()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	exec := NewExecutor(mockDescriber)

	// Set the executor.
	SetDefaultExecutor(exec)
	assert.Equal(t, exec, defaultExecutor)
}

func TestGetDefaultExecutor(t *testing.T) {
	// Save original executor.
	originalExecutor := defaultExecutor
	defer func() {
		defaultExecutor = originalExecutor
	}()

	// Test when nil.
	defaultExecutor = nil
	assert.Nil(t, GetDefaultExecutor())

	// Test when set.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	exec := NewExecutor(mockDescriber)
	defaultExecutor = exec

	result := GetDefaultExecutor()
	assert.Equal(t, exec, result)
}

func TestGetComponentOutputs_PanicWhenNoExecutor(t *testing.T) {
	// Save original executor.
	originalExecutor := defaultExecutor
	defer func() {
		defaultExecutor = originalExecutor
	}()

	defaultExecutor = nil

	atmosConfig := &schema.AtmosConfiguration{}

	assert.PanicsWithValue(t,
		"output.SetDefaultExecutor must be called before GetComponentOutputs",
		func() {
			_, _ = GetComponentOutputs(atmosConfig, "component", "stack", false)
		},
	)
}

func TestExecuteWithSections_PanicWhenNoExecutor(t *testing.T) {
	// Save original executor.
	originalExecutor := defaultExecutor
	defer func() {
		defaultExecutor = originalExecutor
	}()

	defaultExecutor = nil

	atmosConfig := &schema.AtmosConfiguration{}
	sections := map[string]any{}

	assert.PanicsWithValue(t,
		"output.SetDefaultExecutor must be called before ExecuteWithSections",
		func() {
			_, _ = ExecuteWithSections(atmosConfig, "component", "stack", sections, nil)
		},
	)
}

func TestGetOutput_PanicWhenNoExecutor(t *testing.T) {
	// Save original executor.
	originalExecutor := defaultExecutor
	defer func() {
		defaultExecutor = originalExecutor
	}()

	defaultExecutor = nil

	atmosConfig := &schema.AtmosConfiguration{}

	assert.PanicsWithValue(t,
		"output.SetDefaultExecutor must be called before GetOutput",
		func() {
			_, _, _ = GetOutput(atmosConfig, "stack", "component", "output", false, nil, nil)
		},
	)
}

func TestGetComponentOutputsWithExecutor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)

	// Set up the mock to return sections for a disabled component.
	sections := map[string]any{
		"vars": map[string]any{
			"enabled": false,
		},
	}
	mockDescriber.EXPECT().
		DescribeComponent(gomock.Any()).
		Return(sections, nil)

	exec := NewExecutor(mockDescriber)
	atmosConfig := &schema.AtmosConfiguration{
		Logs: schema.Logs{Level: "debug"},
	}

	outputs, err := GetComponentOutputsWithExecutor(exec, atmosConfig, "component", "stack", false)
	require.NoError(t, err)
	assert.Empty(t, outputs)
}
