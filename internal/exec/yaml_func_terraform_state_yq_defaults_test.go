package exec

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestTerraformState_YqDefaultWhenBackendReturnsNil verifies that YQ default
// values work when the backend returns nil (component not provisioned).
// The mock returns the ErrTerraformStateNotProvisioned sentinel error to simulate
// the real behavior when a component is not provisioned.
func TestTerraformState_YqDefaultWhenBackendReturnsNil(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	// The YQ expression after CSV parsing will have the inner quotes preserved.
	// Input: ".bucket_name // ""default-bucket"""
	// After CSV parse: .bucket_name // "default-bucket"
	expectedYqExpr := `.bucket_name // "default-bucket"`

	// Mock returns ErrTerraformStateNotProvisioned - simulating component not provisioned.
	// This is the sentinel error that triggers YQ default evaluation.
	mockStateGetter.EXPECT().
		GetState(
			atmosConfig,
			gomock.Any(), // yamlFunc (full function call string)
			"test-stack",
			"vpc",
			expectedYqExpr, // YQ expression with default
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, fmt.Errorf("%w for component `vpc` in stack `test-stack`", errUtils.ErrTerraformStateNotProvisioned)).
		Times(1)

	// Input using YAML-style double quotes for escaping (like the fixture files).
	// In YAML: !terraform.state vpc test-stack ".bucket_name // ""default-bucket"""
	// The CSV parser handles the outer quotes and unescapes inner double quotes.
	input := schema.AtmosSectionMapType{
		"bucket": `!terraform.state vpc test-stack ".bucket_name // ""default-bucket"""`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "default-bucket", result["bucket"])
}

// TestTerraformState_APIErrorReturnsError verifies that API errors (non-recoverable)
// properly return an error instead of using YQ defaults.
// API errors like S3 timeouts should cause the command to fail, not silently use defaults.
func TestTerraformState_APIErrorReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	expectedYqExpr := `.bucket_name // "default-bucket"`

	// Mock returns a non-recoverable API error - S3 failure.
	// This type of error should NOT use YQ defaults.
	mockStateGetter.EXPECT().
		GetState(
			atmosConfig,
			gomock.Any(),
			"test-stack",
			"vpc",
			expectedYqExpr,
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, fmt.Errorf("S3 GetObject timeout after 30s")).
		Times(1)

	input := schema.AtmosSectionMapType{
		"bucket": `!terraform.state vpc test-stack ".bucket_name // ""default-bucket"""`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	// API errors should propagate as errors, not use defaults.
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "S3 GetObject timeout")
}

// TestTerraformState_YqDefaultWithListFallback verifies that YQ default
// values work with list fallback expressions.
func TestTerraformState_YqDefaultWithListFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	expectedYqExpr := `.subnets // ["subnet-1", "subnet-2"]`

	// Mock returns ErrTerraformStateNotProvisioned - triggers YQ default evaluation.
	mockStateGetter.EXPECT().
		GetState(
			atmosConfig,
			gomock.Any(),
			"test-stack",
			"vpc",
			expectedYqExpr,
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, fmt.Errorf("%w for component `vpc` in stack `test-stack`", errUtils.ErrTerraformStateNotProvisioned)).
		Times(1)

	input := schema.AtmosSectionMapType{
		"subnets": `!terraform.state vpc test-stack ".subnets // [""subnet-1"", ""subnet-2""]"`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, []any{"subnet-1", "subnet-2"}, result["subnets"])
}

// TestTerraformState_NoDefaultExpressionReturnsError verifies that expressions
// WITHOUT YQ defaults return an error when the component is not provisioned.
// This is expected - without a default, the user must handle the error.
func TestTerraformState_NoDefaultExpressionReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	// Mock returns ErrTerraformStateNotProvisioned.
	// Without a default, this error should propagate.
	mockStateGetter.EXPECT().
		GetState(
			atmosConfig,
			gomock.Any(),
			"test-stack",
			"vpc",
			".bucket_name", // No default expression (no //)
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, fmt.Errorf("%w for component `vpc` in stack `test-stack`", errUtils.ErrTerraformStateNotProvisioned)).
		Times(1)

	input := schema.AtmosSectionMapType{
		"bucket": "!terraform.state vpc test-stack .bucket_name",
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	// Without a default, the error should propagate.
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, errUtils.ErrTerraformStateNotProvisioned)
}
