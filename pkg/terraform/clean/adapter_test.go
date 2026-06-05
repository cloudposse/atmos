package clean

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestNewExecAdapter tests the adapter constructor.
func TestNewExecAdapter(t *testing.T) {
	adapter := NewExecAdapter(
		func(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo) (schema.ConfigAndStacksInfo, error) {
			return info, nil
		},
		func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components []string) (map[string]any, error) {
			return nil, nil
		},
		func(componentSection map[string]any) []string { return nil },
		func(basePath string, componentPaths []string, patterns []string) ([]Directory, error) {
			return nil, nil
		},
		func(info *schema.ConfigAndStacksInfo) string { return "" },
		func(info *schema.ConfigAndStacksInfo) string { return "" },
		func(stacksMap map[string]any) []string { return nil },
	)

	require.NotNil(t, adapter)
}

// TestExecAdapter_ProcessStacks tests the ProcessStacks delegation.
func TestExecAdapter_ProcessStacks(t *testing.T) {
	called := false
	expectedInfo := schema.ConfigAndStacksInfo{
		Component: "vpc",
		Stack:     "dev",
	}
	returnedInfo := schema.ConfigAndStacksInfo{
		Component:      "vpc",
		Stack:          "dev",
		FinalComponent: "vpc-resolved",
	}

	adapter := NewExecAdapter(
		func(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo) (schema.ConfigAndStacksInfo, error) {
			called = true
			assert.Equal(t, expectedInfo.Component, info.Component)
			assert.Equal(t, expectedInfo.Stack, info.Stack)
			return returnedInfo, nil
		},
		nil, nil, nil, nil, nil, nil,
	)

	atmosConfig := &schema.AtmosConfiguration{}
	result, err := adapter.ProcessStacks(atmosConfig, expectedInfo)

	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, returnedInfo.FinalComponent, result.FinalComponent)
}

// TestExecAdapter_ProcessStacks_Error tests error propagation.
func TestExecAdapter_ProcessStacks_Error(t *testing.T) {
	expectedErr := errors.New("process stacks error")

	adapter := NewExecAdapter(
		func(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo) (schema.ConfigAndStacksInfo, error) {
			return schema.ConfigAndStacksInfo{}, expectedErr
		},
		nil, nil, nil, nil, nil, nil,
	)

	_, err := adapter.ProcessStacks(&schema.AtmosConfiguration{}, schema.ConfigAndStacksInfo{})
	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

// TestExecAdapter_ExecuteDescribeStacks tests the ExecuteDescribeStacks delegation.
func TestExecAdapter_ExecuteDescribeStacks(t *testing.T) {
	called := false
	expectedStack := "dev"
	expectedComponents := []string{"vpc", "rds"}
	returnedMap := map[string]any{
		"dev": map[string]any{"components": map[string]any{}},
	}

	adapter := NewExecAdapter(
		nil,
		func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components []string) (map[string]any, error) {
			called = true
			assert.Equal(t, expectedStack, filterByStack)
			assert.Equal(t, expectedComponents, components)
			return returnedMap, nil
		},
		nil, nil, nil, nil, nil,
	)

	result, err := adapter.ExecuteDescribeStacks(&schema.AtmosConfiguration{}, expectedStack, expectedComponents)

	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, returnedMap, result)
}

// TestExecAdapter_GetGenerateFilenamesForComponent tests the delegation.
func TestExecAdapter_GetGenerateFilenamesForComponent(t *testing.T) {
	called := false
	expectedSection := map[string]any{"generate": map[string]any{"file.tf": "content"}}
	expectedFilenames := []string{"file.tf", "backend.tf.json"}

	adapter := NewExecAdapter(
		nil, nil,
		func(componentSection map[string]any) []string {
			called = true
			assert.Equal(t, expectedSection, componentSection)
			return expectedFilenames
		},
		nil, nil, nil, nil,
	)

	result := adapter.GetGenerateFilenamesForComponent(expectedSection)

	assert.True(t, called)
	assert.Equal(t, expectedFilenames, result)
}

// TestExecAdapter_CollectComponentsDirectoryObjects tests the delegation.
func TestExecAdapter_CollectComponentsDirectoryObjects(t *testing.T) {
	called := false
	expectedBasePath := "/base/path"
	expectedPaths := []string{"vpc", "rds"}
	expectedPatterns := []string{".terraform", "*.tfvars.json"}
	returnedDirs := []Directory{
		{Name: "vpc", FullPath: "/base/path/vpc"},
	}

	adapter := NewExecAdapter(
		nil, nil, nil,
		func(basePath string, componentPaths []string, patterns []string) ([]Directory, error) {
			called = true
			assert.Equal(t, expectedBasePath, basePath)
			assert.Equal(t, expectedPaths, componentPaths)
			assert.Equal(t, expectedPatterns, patterns)
			return returnedDirs, nil
		},
		nil, nil, nil,
	)

	result, err := adapter.CollectComponentsDirectoryObjects(expectedBasePath, expectedPaths, expectedPatterns)

	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, returnedDirs, result)
}

// TestExecAdapter_ConstructTerraformComponentVarfileName tests the delegation.
func TestExecAdapter_ConstructTerraformComponentVarfileName(t *testing.T) {
	called := false
	expectedVarfile := "dev-vpc.tfvars.json"

	adapter := NewExecAdapter(
		nil, nil, nil, nil,
		func(info *schema.ConfigAndStacksInfo) string {
			called = true
			assert.NotNil(t, info)
			return expectedVarfile
		},
		nil, nil,
	)

	info := &schema.ConfigAndStacksInfo{Component: "vpc", Stack: "dev"}
	result := adapter.ConstructTerraformComponentVarfileName(info)

	assert.True(t, called)
	assert.Equal(t, expectedVarfile, result)
}

// TestExecAdapter_ConstructTerraformComponentPlanfileName tests the delegation.
func TestExecAdapter_ConstructTerraformComponentPlanfileName(t *testing.T) {
	called := false
	expectedPlanfile := "dev-vpc.planfile"

	adapter := NewExecAdapter(
		nil, nil, nil, nil, nil,
		func(info *schema.ConfigAndStacksInfo) string {
			called = true
			assert.NotNil(t, info)
			return expectedPlanfile
		},
		nil,
	)

	info := &schema.ConfigAndStacksInfo{Component: "vpc", Stack: "dev"}
	result := adapter.ConstructTerraformComponentPlanfileName(info)

	assert.True(t, called)
	assert.Equal(t, expectedPlanfile, result)
}

// TestExecAdapter_GetAllStacksComponentsPaths tests the delegation.
func TestExecAdapter_GetAllStacksComponentsPaths(t *testing.T) {
	called := false
	expectedStacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{"component": "vpc"},
				},
			},
		},
	}
	expectedPaths := []string{"vpc", "rds"}

	adapter := NewExecAdapter(
		nil, nil, nil, nil, nil, nil,
		func(stacksMap map[string]any) []string {
			called = true
			assert.Equal(t, expectedStacksMap, stacksMap)
			return expectedPaths
		},
	)

	result := adapter.GetAllStacksComponentsPaths(expectedStacksMap)

	assert.True(t, called)
	assert.Equal(t, expectedPaths, result)
}

// TestExecAdapter_ImplementsStackProcessor verifies the adapter implements the interface.
func TestExecAdapter_ImplementsStackProcessor(t *testing.T) {
	adapter := NewExecAdapter(
		func(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo) (schema.ConfigAndStacksInfo, error) {
			return info, nil
		},
		func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components []string) (map[string]any, error) {
			return nil, nil
		},
		func(componentSection map[string]any) []string { return nil },
		func(basePath string, componentPaths []string, patterns []string) ([]Directory, error) {
			return nil, nil
		},
		func(info *schema.ConfigAndStacksInfo) string { return "" },
		func(info *schema.ConfigAndStacksInfo) string { return "" },
		func(stacksMap map[string]any) []string { return nil },
	)

	// This will fail to compile if ExecAdapter doesn't implement StackProcessor.
	var _ StackProcessor = adapter
	assert.NotNil(t, adapter)
}
