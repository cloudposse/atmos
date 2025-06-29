package exec

import (
	"errors"
	"testing"

	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewDescribeDependentsExec(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	exec := NewDescribeDependentsExec(atmosConfig)

	assert.NotNil(t, exec)
	assert.Equal(t, atmosConfig, exec.(*describeDependentsExec).atmosConfig)
	assert.NotNil(t, exec.(*describeDependentsExec).executeDescribeDependents)
	assert.NotNil(t, exec.(*describeDependentsExec).newPageCreator)
	assert.NotNil(t, exec.(*describeDependentsExec).isTTYSupportForStdout)
}

func TestDescribeDependentsExec_Execute_Success_NoQuery(t *testing.T) {
	// Setup
	atmosConfig := &schema.AtmosConfiguration{}
	dependents := []schema.Dependent{
		{Component: "comp1", Stack: "stack1"},
		{Component: "comp2", Stack: "stack2"},
	}

	// Mock functions
	mockExecuteDescribeDependents := func(config schema.AtmosConfiguration, component, stack string, includeSettings bool) ([]schema.Dependent, error) {
		assert.Equal(t, "test-component", component)
		assert.Equal(t, "test-stack", stack)
		assert.False(t, includeSettings)
		return dependents, nil
	}

	mockIsTTYSupportForStdout := func() bool {
		return true
	}

	ctrl := gomock.NewController(t)
	mockPageCreator := pager.NewMockPageCreator(ctrl)
	exec := &describeDependentsExec{
		executeDescribeDependents: mockExecuteDescribeDependents,
		evaluateYqExpression: func(config *schema.AtmosConfiguration, data any, query string) (any, error) {
			assert.Equal(t, atmosConfig, config)
			assert.Equal(t, dependents, data)
			assert.Equal(t, "", query) // No query provided
			return dependents, nil
		},
		atmosConfig:           atmosConfig,
		newPageCreator:        mockPageCreator,
		isTTYSupportForStdout: mockIsTTYSupportForStdout,
	}

	props := &DescribeDependentsExecProps{
		Component: "test-component",
		Stack:     "test-stack",
		Format:    "yaml",
		File:      "output.yaml",
		Query:     "", // No query
	}

	// Execute
	err := exec.Execute(props)

	// Assert
	assert.NoError(t, err)
}

func TestDescribeDependentsExec_Execute_Success_WithQuery(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	newMockPageCreator := pager.NewMockPageCreator(ctrl)
	atmosConfig := &schema.AtmosConfiguration{}
	dependents := []schema.Dependent{
		{Component: "comp1", Stack: "stack1"},
		{Component: "comp2", Stack: "stack2"},
	}
	queryResult := map[string]string{"filtered": "result"}

	mockEvaluateYqExpression := func(config *schema.AtmosConfiguration, data any, query string) (any, error) {
		assert.Equal(t, atmosConfig, config)
		assert.Equal(t, dependents, data)
		assert.Equal(t, ".name", query)
		return queryResult, nil
	}

	mockIsTTYSupportForStdout := func() bool {
		return false
	}

	exec := &describeDependentsExec{
		atmosConfig: atmosConfig,
		executeDescribeDependents: func(atmosConfig schema.AtmosConfiguration, component, stack string, includeSettings bool) ([]schema.Dependent, error) {
			return dependents, nil
		},
		newPageCreator:        newMockPageCreator,
		isTTYSupportForStdout: mockIsTTYSupportForStdout,
		evaluateYqExpression:  mockEvaluateYqExpression,
	}

	props := &DescribeDependentsExecProps{
		Component: "test-component",
		Stack:     "test-stack",
		Format:    "json",
		File:      "",
		Query:     ".name",
	}

	// Execute
	err := exec.Execute(props)

	// Assert
	assert.NoError(t, err)
}

func TestDescribeDependentsExec_Execute_ExecuteDescribeDependentsError(t *testing.T) {
	// Setup
	atmosConfig := &schema.AtmosConfiguration{}
	expectedError := errors.New("execute describe dependents failed")
	pagerMock := pager.NewMockPageCreator(gomock.NewController(t))
	exec := &describeDependentsExec{
		atmosConfig: atmosConfig,
		executeDescribeDependents: func(atmosConfig schema.AtmosConfiguration, component, stack string, includeSettings bool) ([]schema.Dependent, error) {
			return nil, expectedError
		},
		newPageCreator:        pagerMock,
		isTTYSupportForStdout: func() bool { return false },
	}

	props := &DescribeDependentsExecProps{
		Component: "test-component",
		Stack:     "test-stack",
		Format:    "yaml",
		File:      "",
		Query:     "",
	}

	// Execute
	err := exec.Execute(props)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestDescribeDependentsExec_Execute_YqExpressionError(t *testing.T) {
	// Setup
	atmosConfig := &schema.AtmosConfiguration{}
	ctrl := gomock.NewController(t)
	mockPageCreator := pager.NewMockPageCreator(ctrl)
	dependents := []schema.Dependent{
		{Component: "comp1", Stack: "stack1"},
	}
	expectedError := errors.New("yq expression evaluation failed")

	mockEvaluateYqExpression := func(config *schema.AtmosConfiguration, data any, query string) (any, error) {
		return nil, expectedError
	}

	exec := &describeDependentsExec{
		atmosConfig: atmosConfig,
		executeDescribeDependents: func(atmosConfig schema.AtmosConfiguration, component, stack string, includeSettings bool) ([]schema.Dependent, error) {
			return dependents, nil
		},
		newPageCreator: mockPageCreator,
		isTTYSupportForStdout: func() bool {
			return false
		},
		evaluateYqExpression: mockEvaluateYqExpression,
	}

	props := &DescribeDependentsExecProps{
		Component: "test-component",
		Stack:     "test-stack",
		Format:    "yaml",
		File:      "",
		Query:     ".invalid",
	}

	// Execute
	err := exec.Execute(props)

	// Assert
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)
}

func TestDescribeDependentsExec_Execute_EmptyDependents(t *testing.T) {
	// Setup
	atmosConfig := &schema.AtmosConfiguration{}
	dependents := []schema.Dependent{}

	mockExecuteDescribeDependents := func(config schema.AtmosConfiguration, component, stack string, includeSettings bool) ([]schema.Dependent, error) {
		return dependents, nil
	}

	mockIsTTYSupportForStdout := func() bool {
		return false
	}

	pagerMock := pager.NewMockPageCreator(gomock.NewController(t))
	exec := &describeDependentsExec{
		atmosConfig:               atmosConfig,
		executeDescribeDependents: mockExecuteDescribeDependents,
		newPageCreator:            pagerMock,
		isTTYSupportForStdout:     mockIsTTYSupportForStdout,
	}

	props := &DescribeDependentsExecProps{
		Component: "nonexistent-component",
		Stack:     "nonexistent-stack",
		Format:    "json",
		File:      "",
		Query:     "",
	}

	// Execute
	err := exec.Execute(props)

	// Assert
	assert.NoError(t, err)
}

func TestDescribeDependentsExec_Execute_DifferentFormatsAndFiles(t *testing.T) {
	testCases := []struct {
		name     string
		format   string
		file     string
		expected string
	}{
		{
			name:     "JSON format with file",
			format:   "json",
			file:     "output.json",
			expected: "Dependents of 'comp' in stack 'stack'",
		},
		{
			name:     "YAML format without file",
			format:   "yaml",
			file:     "",
			expected: "Dependents of 'comp' in stack 'stack'",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}
			dependents := []schema.Dependent{{Component: "comp1", Stack: "stack1"}}
			pagerMock := pager.NewMockPageCreator(gomock.NewController(t))
			// Mock functions
			mockExecuteDescribeDependents := func(config schema.AtmosConfiguration, component, stack string, includeSettings bool) ([]schema.Dependent, error) {
				return dependents, nil
			}

			mockIsTTYSupportForStdout := func() bool { return false }

			exec := &describeDependentsExec{
				atmosConfig:               atmosConfig,
				executeDescribeDependents: mockExecuteDescribeDependents,
				newPageCreator:            pagerMock,
				isTTYSupportForStdout:     mockIsTTYSupportForStdout,
			}

			props := &DescribeDependentsExecProps{
				Component: "comp",
				Stack:     "stack",
				Format:    tc.format,
				File:      tc.file,
				Query:     "",
			}

			err := exec.Execute(props)
			assert.NoError(t, err)
		})
	}
}
