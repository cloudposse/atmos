package list

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestProcessInstancesWithDeps_Success tests the happy path where ExecuteDescribeStacks succeeds.
func TestProcessInstancesWithDeps_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStacksProcessor := e.NewMockStacksProcessor(ctrl)
	atmosConfig := &schema.AtmosConfiguration{}

	// Mock stacks map with two components in one stack.
	stacksMap := map[string]interface{}{
		"dev": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc": map[string]interface{}{
						"metadata": map[string]interface{}{"type": "real"},
						"vars":     map[string]interface{}{"region": "us-east-1"},
					},
					"eks": map[string]interface{}{
						"metadata": map[string]interface{}{"type": "real"},
						"vars":     map[string]interface{}{"cluster_name": "test"},
					},
				},
			},
		},
	}

	mockStacksProcessor.EXPECT().
		ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, true, false, false, nil, nil).
		Return(stacksMap, nil)

	instances, err := processInstancesWithDeps(atmosConfig, mockStacksProcessor, nil, true, false)

	assert.NoError(t, err)
	assert.Len(t, instances, 2)
	// Instances should be sorted by stack then component (alphabetically: eks before vpc).
	assert.Equal(t, "eks", instances[0].Component)
	assert.Equal(t, "dev", instances[0].Stack)
	assert.Equal(t, "vpc", instances[1].Component)
	assert.Equal(t, "dev", instances[1].Stack)
}

// TestProcessInstancesWithDeps_ExecuteDescribeStacksError tests error handling when ExecuteDescribeStacks fails.
func TestProcessInstancesWithDeps_ExecuteDescribeStacksError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStacksProcessor := e.NewMockStacksProcessor(ctrl)
	atmosConfig := &schema.AtmosConfiguration{}
	expectedErr := errors.New("failed to read stack files")

	mockStacksProcessor.EXPECT().
		ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, true, false, false, nil, nil).
		Return(nil, expectedErr)

	instances, err := processInstancesWithDeps(atmosConfig, mockStacksProcessor, nil, true, false)

	assert.Error(t, err)
	assert.Nil(t, instances)
	assert.True(t, errors.Is(err, errUtils.ErrExecuteDescribeStacks))
	assert.ErrorContains(t, err, "failed to read stack files")
}

// TestProcessInstancesWithDeps_EmptyStacksMap tests handling of empty stacks map.
func TestProcessInstancesWithDeps_EmptyStacksMap(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStacksProcessor := e.NewMockStacksProcessor(ctrl)
	atmosConfig := &schema.AtmosConfiguration{}
	stacksMap := map[string]interface{}{}

	mockStacksProcessor.EXPECT().
		ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, true, false, false, nil, nil).
		Return(stacksMap, nil)

	instances, err := processInstancesWithDeps(atmosConfig, mockStacksProcessor, nil, true, false)

	assert.NoError(t, err)
	assert.Empty(t, instances)
}

// TestProcessInstancesWithDeps_MultipleStacks tests processing multiple stacks.
func TestProcessInstancesWithDeps_MultipleStacks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStacksProcessor := e.NewMockStacksProcessor(ctrl)
	atmosConfig := &schema.AtmosConfiguration{}

	// Mock stacks map with multiple stacks.
	stacksMap := map[string]interface{}{
		"dev": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc": map[string]interface{}{
						"metadata": map[string]interface{}{"type": "real"},
					},
				},
			},
		},
		"prod": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc": map[string]interface{}{
						"metadata": map[string]interface{}{"type": "real"},
					},
				},
			},
		},
		"staging": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"app": map[string]interface{}{
						"metadata": map[string]interface{}{"type": "real"},
					},
				},
			},
		},
	}

	mockStacksProcessor.EXPECT().
		ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, true, false, false, nil, nil).
		Return(stacksMap, nil)

	instances, err := processInstancesWithDeps(atmosConfig, mockStacksProcessor, nil, true, false)

	assert.NoError(t, err)
	assert.Len(t, instances, 3)
	// Verify sorting: dev before prod before staging, and within same stack, alphabetically.
	assert.Equal(t, "vpc", instances[0].Component)
	assert.Equal(t, "dev", instances[0].Stack)
	assert.Equal(t, "vpc", instances[1].Component)
	assert.Equal(t, "prod", instances[1].Stack)
	assert.Equal(t, "app", instances[2].Component)
	assert.Equal(t, "staging", instances[2].Stack)
}

// TestProcessInstancesWithDeps_AbstractComponentsFiltered tests that abstract components are filtered out.
func TestProcessInstancesWithDeps_AbstractComponentsFiltered(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStacksProcessor := e.NewMockStacksProcessor(ctrl)
	atmosConfig := &schema.AtmosConfiguration{}

	// Mock stacks map with abstract and real components.
	stacksMap := map[string]interface{}{
		"dev": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc-base": map[string]interface{}{
						"metadata": map[string]interface{}{"type": "abstract"},
					},
					"vpc": map[string]interface{}{
						"metadata": map[string]interface{}{"type": "real"},
					},
				},
			},
		},
	}

	mockStacksProcessor.EXPECT().
		ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, true, false, false, nil, nil).
		Return(stacksMap, nil)

	instances, err := processInstancesWithDeps(atmosConfig, mockStacksProcessor, nil, true, false)

	assert.NoError(t, err)
	assert.Len(t, instances, 1)
	assert.Equal(t, "vpc", instances[0].Component)
	assert.NotContains(t, instances, schema.Instance{Component: "vpc-base", Stack: "dev"})
}

// TestProcessInstancesWithDeps_InvalidStackStructure tests handling of invalid stack structure.
func TestProcessInstancesWithDeps_InvalidStackStructure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStacksProcessor := e.NewMockStacksProcessor(ctrl)
	atmosConfig := &schema.AtmosConfiguration{}

	// Mock stacks map with invalid structure (missing components section).
	stacksMap := map[string]interface{}{
		"dev": map[string]interface{}{
			"invalid_key": "invalid_value",
		},
		"prod": map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc": map[string]interface{}{
						"metadata": map[string]interface{}{"type": "real"},
					},
				},
			},
		},
	}

	mockStacksProcessor.EXPECT().
		ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, true, false, false, nil, nil).
		Return(stacksMap, nil)

	instances, err := processInstancesWithDeps(atmosConfig, mockStacksProcessor, nil, true, false)

	assert.NoError(t, err)
	// Only prod stack should be processed successfully.
	assert.Len(t, instances, 1)
	assert.Equal(t, "vpc", instances[0].Component)
	assert.Equal(t, "prod", instances[0].Stack)
}

// TestProcessInstancesWithDeps_PropagatesTemplateAndFunctionFlags verifies that
// the processTemplates and processYamlFunctions arguments are forwarded to
// ExecuteDescribeStacks unchanged. Regression guard: the arguments used to be
// hardcoded to (true, false) before the --process-templates / --process-functions
// flags were added; this test ensures all four combinations reach the processor.
//
// See docs/fixes/2026-04-24-list-instances-per-component-auth.md and the
// companion flag-rollout commit.
func TestProcessInstancesWithDeps_PropagatesTemplateAndFunctionFlags(t *testing.T) {
	tests := []struct {
		name                 string
		processTemplates     bool
		processYamlFunctions bool
	}{
		{"templates_on_functions_off", true, false},
		{"templates_on_functions_on", true, true},
		{"templates_off_functions_off", false, false},
		{"templates_off_functions_on", false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStacksProcessor := e.NewMockStacksProcessor(ctrl)
			atmosConfig := &schema.AtmosConfiguration{}

			// Empty stacks map — the test only cares about the arguments,
			// not the returned data.
			mockStacksProcessor.EXPECT().
				ExecuteDescribeStacks(
					atmosConfig,             // atmosConfig
					"",                      // filterByStack
					nil,                     // components
					nil,                     // componentTypes
					nil,                     // sections
					false,                   // ignoreMissingFiles
					tc.processTemplates,     // processTemplates
					tc.processYamlFunctions, // processYamlFunctions
					false,                   // includeEmptyStacks
					nil,                     // skip
					nil,                     // authManager
				).
				Return(map[string]interface{}{}, nil)

			_, err := processInstancesWithDeps(atmosConfig, mockStacksProcessor, nil, tc.processTemplates, tc.processYamlFunctions)
			assert.NoError(t, err)
		})
	}
}
