package utils

import (
	"errors"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

// TestCheckComponentExists_EmptyComponentName tests early return at utils.go:18-20.
func TestCheckComponentExists_EmptyComponentName(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	result := CheckComponentExists(atmosConfig, "")
	assert.False(t, result)
}

// TestCheckComponentExists_ExecuteDescribeStacksError tests error path at utils.go:23-26.
func TestCheckComponentExists_ExecuteDescribeStacksError(t *testing.T) {
	tests.SkipOnDarwinARM64(t, "gomonkey causes SIGBUS panics on ARM64 macOS due to memory protection")

	atmosConfig := &schema.AtmosConfiguration{}

	// Mock ExecuteDescribeStacks to return an error
	patches := gomonkey.ApplyFunc(e.ExecuteDescribeStacks,
		func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string) (map[string]any, error) {
			return nil, errors.New("simulated ExecuteDescribeStacks error")
		})
	defer patches.Reset()

	result := CheckComponentExists(atmosConfig, "test-component")
	assert.False(t, result) // Should return false on error
}

// TestCheckComponentExists_EmptyStacksMap tests path at utils.go:29.
func TestCheckComponentExists_EmptyStacksMap(t *testing.T) {
	tests.SkipOnDarwinARM64(t, "gomonkey causes SIGBUS panics on ARM64 macOS due to memory protection")
	atmosConfig := &schema.AtmosConfiguration{}

	// Mock ExecuteDescribeStacks to return empty map
	patches := gomonkey.ApplyFunc(e.ExecuteDescribeStacks,
		func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string) (map[string]any, error) {
			return map[string]any{}, nil
		})
	defer patches.Reset()

	result := CheckComponentExists(atmosConfig, "test-component")
	assert.False(t, result) // Should return false when no stacks
}

// TestCheckComponentExists_InvalidStackData tests type assertion at utils.go:30-33.
func TestCheckComponentExists_InvalidStackData(t *testing.T) {
	tests.SkipOnDarwinARM64(t, "gomonkey causes SIGBUS panics on ARM64 macOS due to memory protection")
	atmosConfig := &schema.AtmosConfiguration{}

	// Mock ExecuteDescribeStacks to return invalid stack data
	patches := gomonkey.ApplyFunc(e.ExecuteDescribeStacks,
		func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string) (map[string]any, error) {
			return map[string]any{
				"stack1": "invalid-not-a-map", // Invalid type
			}, nil
		})
	defer patches.Reset()

	result := CheckComponentExists(atmosConfig, "test-component")
	assert.False(t, result) // Should skip invalid stack data
}

// TestCheckComponentExists_NoComponentsKey tests path at utils.go:35-38.
func TestCheckComponentExists_NoComponentsKey(t *testing.T) {
	tests.SkipOnDarwinARM64(t, "gomonkey causes SIGBUS panics on ARM64 macOS due to memory protection")
	atmosConfig := &schema.AtmosConfiguration{}

	// Mock ExecuteDescribeStacks to return stack without components key
	patches := gomonkey.ApplyFunc(e.ExecuteDescribeStacks,
		func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string) (map[string]any, error) {
			return map[string]any{
				"stack1": map[string]interface{}{
					"other_key": "value",
					// No "components" key
				},
			}, nil
		})
	defer patches.Reset()

	result := CheckComponentExists(atmosConfig, "test-component")
	assert.False(t, result)
}

// TestCheckComponentExists_InvalidComponentsType tests type assertion at utils.go:35-38.
func TestCheckComponentExists_InvalidComponentsType(t *testing.T) {
	tests.SkipOnDarwinARM64(t, "gomonkey causes SIGBUS panics on ARM64 macOS due to memory protection")
	atmosConfig := &schema.AtmosConfiguration{}

	// Mock ExecuteDescribeStacks to return invalid components type
	patches := gomonkey.ApplyFunc(e.ExecuteDescribeStacks,
		func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string) (map[string]any, error) {
			return map[string]any{
				"stack1": map[string]interface{}{
					"components": "invalid-not-a-map",
				},
			}, nil
		})
	defer patches.Reset()

	result := CheckComponentExists(atmosConfig, "test-component")
	assert.False(t, result)
}

// TestCheckComponentExists_InvalidComponentTypeMap tests type assertion at utils.go:41-44.
func TestCheckComponentExists_InvalidComponentTypeMap(t *testing.T) {
	tests.SkipOnDarwinARM64(t, "gomonkey causes SIGBUS panics on ARM64 macOS due to memory protection")
	atmosConfig := &schema.AtmosConfiguration{}

	// Mock ExecuteDescribeStacks to return invalid component type map
	patches := gomonkey.ApplyFunc(e.ExecuteDescribeStacks,
		func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string) (map[string]any, error) {
			return map[string]any{
				"stack1": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": "invalid-not-a-map", // Invalid type
					},
				},
			}, nil
		})
	defer patches.Reset()

	result := CheckComponentExists(atmosConfig, "test-component")
	assert.False(t, result)
}

// TestCheckComponentExists_ComponentNotFound tests path at utils.go:46-49 (not found).
func TestCheckComponentExists_ComponentNotFound(t *testing.T) {
	tests.SkipOnDarwinARM64(t, "gomonkey causes SIGBUS panics on ARM64 macOS due to memory protection")
	atmosConfig := &schema.AtmosConfiguration{}

	// Mock ExecuteDescribeStacks to return stacks without the target component
	patches := gomonkey.ApplyFunc(e.ExecuteDescribeStacks,
		func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string) (map[string]any, error) {
			return map[string]any{
				"stack1": map[string]interface{}{
					"components": map[string]interface{}{
						"terraform": map[string]interface{}{
							"vpc":   map[string]interface{}{"var1": "value1"},
							"eks":   map[string]interface{}{"var2": "value2"},
							"other": map[string]interface{}{"var3": "value3"},
							// "test-component" is NOT here
						},
					},
				},
				"stack2": map[string]interface{}{
					"components": map[string]interface{}{
						"helmfile": map[string]interface{}{
							"nginx": map[string]interface{}{"var4": "value4"},
						},
					},
				},
			}, nil
		})

	result := CheckComponentExists(atmosConfig, "test-component")
	patches.Reset()
	assert.False(t, result)
}

// Note: Success path tests (ComponentFound, ComponentFoundInSecondStack, MultipleComponentTypes, MixedValidInvalidStacks)
// are covered by TestCheckComponentExistsLogic integration test which uses actual stack configurations.
// Gomonkey mocking causes interference when running tests sequentially, so we rely on the integration test instead.

// TestCheckComponentExists_EmptyComponentName_NilConfig tests edge case.
func TestCheckComponentExists_EmptyComponentName_NilConfig(t *testing.T) {
	// Even with nil config, empty component name should return false immediately
	result := CheckComponentExists(nil, "")
	assert.False(t, result)
}

// TestCheckComponentExists_RealComponentName_NilConfig tests nil config handling.
func TestCheckComponentExists_RealComponentName_NilConfig(t *testing.T) {
	tests.SkipOnDarwinARM64(t, "gomonkey causes SIGBUS panics on ARM64 macOS due to memory protection")
	// Mock ExecuteDescribeStacks to handle nil config gracefully
	patches := gomonkey.ApplyFunc(e.ExecuteDescribeStacks,
		func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string) (map[string]any, error) {
			// ExecuteDescribeStacks might handle nil config
			return map[string]any{}, nil
		})
	defer patches.Reset()

	result := CheckComponentExists(nil, "nil-config-component")
	assert.False(t, result)
}
