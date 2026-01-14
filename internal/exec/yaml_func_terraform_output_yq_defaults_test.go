package exec

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestTerraformOutput_RecoverableErrorWithDefaultUsesDefault verifies that when
// GetOutput returns a recoverable error (state not provisioned) AND the expression
// has a YQ default, the default is used.
func TestTerraformOutput_RecoverableErrorWithDefaultUsesDefault(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)
	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	expectedYqExpr := `.bucket_name // "default-bucket"`

	// Mock returns a recoverable error - state not provisioned.
	// This is a recoverable error that should use the YQ default.
	mockOutputGetter.EXPECT().
		GetOutput(
			atmosConfig,
			"test-stack",
			"vpc",
			expectedYqExpr,
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, false, fmt.Errorf("component not provisioned: %w", errUtils.ErrTerraformStateNotProvisioned)).
		Times(1)

	input := schema.AtmosSectionMapType{
		"bucket": `!terraform.output vpc test-stack ".bucket_name // ""default-bucket"""`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	// With a YQ default and a recoverable error, the default should be used.
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "default-bucket", result["bucket"])
}

// TestTerraformOutput_APIErrorWithDefaultReturnsError verifies that when GetOutput
// returns a non-recoverable API error, even with a YQ default, the error propagates.
// This ensures that infrastructure/API failures don't silently use defaults.
func TestTerraformOutput_APIErrorWithDefaultReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)
	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	expectedYqExpr := `.bucket_name // "default-bucket"`

	// Mock returns a non-recoverable API error (S3 connection failure).
	// This should NOT use the YQ default - it should return the error.
	mockOutputGetter.EXPECT().
		GetOutput(
			atmosConfig,
			"test-stack",
			"vpc",
			expectedYqExpr,
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, false, fmt.Errorf("S3 connection timeout: %w", errUtils.ErrGetObjectFromS3)).
		Times(1)

	input := schema.AtmosSectionMapType{
		"bucket": `!terraform.output vpc test-stack ".bucket_name // ""default-bucket"""`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	// API errors should propagate even when a YQ default is present.
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "S3 connection timeout")
}

// TestTerraformOutput_YqDefaultWhenOutputNotExists verifies that YQ default
// values work when the output doesn't exist (exists=false).
func TestTerraformOutput_YqDefaultWhenOutputNotExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)
	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	expectedYqExpr := `.bucket_name // "default-bucket"`

	// Mock returns exists=false - output doesn't exist.
	mockOutputGetter.EXPECT().
		GetOutput(
			atmosConfig,
			"test-stack",
			"vpc",
			expectedYqExpr,
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, false, nil). // exists=false, no error
		Times(1)

	input := schema.AtmosSectionMapType{
		"bucket": `!terraform.output vpc test-stack ".bucket_name // ""default-bucket"""`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "default-bucket", result["bucket"])
}

// TestTerraformOutput_YqDefaultWithListFallback verifies that YQ default
// values work with list fallback expressions.
func TestTerraformOutput_YqDefaultWithListFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)
	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	expectedYqExpr := `.subnets // ["subnet-1", "subnet-2"]`

	// Mock returns exists=false - output doesn't exist.
	mockOutputGetter.EXPECT().
		GetOutput(
			atmosConfig,
			"test-stack",
			"vpc",
			expectedYqExpr,
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, false, nil).
		Times(1)

	input := schema.AtmosSectionMapType{
		"subnets": `!terraform.output vpc test-stack ".subnets // [""subnet-1"", ""subnet-2""]"`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, []any{"subnet-1", "subnet-2"}, result["subnets"])
}

// TestTerraformOutput_NoDefaultExpressionReturnsNil verifies that expressions
// WITHOUT YQ defaults still return nil when output doesn't exist.
// This is the expected behavior - no default means nil is acceptable.
func TestTerraformOutput_NoDefaultExpressionReturnsNil(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)
	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	// Mock returns exists=false - output doesn't exist.
	mockOutputGetter.EXPECT().
		GetOutput(
			atmosConfig,
			"test-stack",
			"vpc",
			".bucket_name", // No default expression (no //)
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, false, nil).
		Times(1)

	input := schema.AtmosSectionMapType{
		"bucket": "!terraform.output vpc test-stack .bucket_name",
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Without a default, nil is the expected result (backward compatible).
	assert.Nil(t, result["bucket"])
}

// TestTerraformOutput_YqDefaultWhenValueIsNilButExists verifies that when
// the output exists but has a nil value, the value is returned (nil is valid).
// YQ evaluation happens at this level, and YQ's // operator handles null.
func TestTerraformOutput_YqDefaultWhenValueIsNilButExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)
	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	expectedYqExpr := `.bucket_name // "default-bucket"`

	// Mock returns nil value but exists=true (output is explicitly null in terraform).
	// When the output exists with a nil value, that's valid - return nil.
	mockOutputGetter.EXPECT().
		GetOutput(
			atmosConfig,
			"test-stack",
			"vpc",
			expectedYqExpr,
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, true, nil). // value=nil, exists=true (terraform null output)
		Times(1)

	input := schema.AtmosSectionMapType{
		"bucket": `!terraform.output vpc test-stack ".bucket_name // ""default-bucket"""`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// When the output exists (even with nil value), return the value.
	// The YQ evaluation already happened in GetOutput.
	assert.Nil(t, result["bucket"])
}

// TestTerraformOutput_APIErrorWithoutDefaultReturnsError verifies that API errors
// without YQ defaults properly return errors.
func TestTerraformOutput_APIErrorWithoutDefaultReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)
	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	// Mock returns error - simulating terraform output failure.
	mockOutputGetter.EXPECT().
		GetOutput(
			atmosConfig,
			"test-stack",
			"vpc",
			".bucket_name", // No default expression
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, false, fmt.Errorf("S3 connection timeout")).
		Times(1)

	input := schema.AtmosSectionMapType{
		"bucket": "!terraform.output vpc test-stack .bucket_name",
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	// Without a default, API errors should propagate.
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "S3 connection timeout")
}

// TestTerraformOutput_OutputNotFoundWithDefaultUsesDefault verifies that when
// GetOutput returns ErrTerraformOutputNotFound (output key doesn't exist in state)
// AND the expression has a YQ default, the default is used.
func TestTerraformOutput_OutputNotFoundWithDefaultUsesDefault(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)
	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	expectedYqExpr := `.missing_output // "fallback-value"`

	// Mock returns ErrTerraformOutputNotFound - the output key doesn't exist.
	mockOutputGetter.EXPECT().
		GetOutput(
			atmosConfig,
			"test-stack",
			"vpc",
			expectedYqExpr,
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, false, fmt.Errorf("output key not found: %w", errUtils.ErrTerraformOutputNotFound)).
		Times(1)

	input := schema.AtmosSectionMapType{
		"value": `!terraform.output vpc test-stack ".missing_output // ""fallback-value"""`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	// With a YQ default and ErrTerraformOutputNotFound, the default should be used.
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "fallback-value", result["value"])
}

// TestTerraformOutput_YqDefaultWithMapFallback verifies that YQ default
// values work with map/object fallback expressions.
func TestTerraformOutput_YqDefaultWithMapFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)
	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	expectedYqExpr := `.tags // {"env": "dev", "team": "platform"}`

	// Mock returns exists=false - output doesn't exist.
	mockOutputGetter.EXPECT().
		GetOutput(
			atmosConfig,
			"test-stack",
			"config",
			expectedYqExpr,
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, false, nil).
		Times(1)

	input := schema.AtmosSectionMapType{
		"tags": `!terraform.output config test-stack ".tags // {""env"": ""dev"", ""team"": ""platform""}"`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Verify the map structure is returned correctly.
	expectedMap := map[string]any{"env": "dev", "team": "platform"}
	assert.Equal(t, expectedMap, result["tags"])
}

// TestTerraformOutput_YqDefaultWithEmptyStringFallback verifies behavior
// when using an empty string as a YQ default value.
// Note: YQ evaluates empty strings to nil in the current implementation.
func TestTerraformOutput_YqDefaultWithEmptyStringFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)
	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	expectedYqExpr := `.optional_value // ""`

	// Mock returns exists=false - output doesn't exist.
	mockOutputGetter.EXPECT().
		GetOutput(
			atmosConfig,
			"test-stack",
			"config",
			expectedYqExpr,
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, false, nil).
		Times(1)

	input := schema.AtmosSectionMapType{
		"optional": `!terraform.output config test-stack ".optional_value // """""`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Note: YQ evaluates empty string default to nil in current implementation.
	// This documents the actual behavior.
	assert.Nil(t, result["optional"])
}

// TestTerraformOutput_YqDefaultWithNumericFallback verifies that
// numeric defaults work correctly.
func TestTerraformOutput_YqDefaultWithNumericFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)
	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	expectedYqExpr := `.port // 8080`

	// Mock returns exists=false - output doesn't exist.
	mockOutputGetter.EXPECT().
		GetOutput(
			atmosConfig,
			"test-stack",
			"config",
			expectedYqExpr,
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, false, nil).
		Times(1)

	input := schema.AtmosSectionMapType{
		"port": `!terraform.output config test-stack ".port // 8080"`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Numeric default should work.
	assert.Equal(t, 8080, result["port"])
}

// TestTerraformOutput_YqDefaultWithBooleanFallback verifies that
// boolean defaults work correctly.
func TestTerraformOutput_YqDefaultWithBooleanFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)
	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	expectedYqExpr := `.enabled // true`

	// Mock returns exists=false - output doesn't exist.
	mockOutputGetter.EXPECT().
		GetOutput(
			atmosConfig,
			"test-stack",
			"config",
			expectedYqExpr,
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, false, nil).
		Times(1)

	input := schema.AtmosSectionMapType{
		"enabled": `!terraform.output config test-stack ".enabled // true"`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Boolean default should work.
	assert.Equal(t, true, result["enabled"])
}
