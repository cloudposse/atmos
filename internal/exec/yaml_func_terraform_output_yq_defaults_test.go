package exec

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestTerraformOutput_APIErrorWithDefaultUsesDefault verifies that when GetOutput
// returns an error AND the expression has a YQ default, the default is used.
func TestTerraformOutput_APIErrorWithDefaultUsesDefault(t *testing.T) {
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

	// Mock returns error - simulating terraform output failure.
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
		Return(nil, false, fmt.Errorf("terraform output failed: state not found")).
		Times(1)

	input := schema.AtmosSectionMapType{
		"bucket": `!terraform.output vpc test-stack ".bucket_name // ""default-bucket"""`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	// With a YQ default and an error, the default should be used.
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "default-bucket", result["bucket"])
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
