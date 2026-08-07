package exec

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	fnparser "github.com/cloudposse/atmos/pkg/function/parser"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestIsRecoverableTerraformError tests the error classification helper function.
func TestIsRecoverableTerraformError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "ErrTerraformStateNotProvisioned is recoverable",
			err:      errUtils.ErrTerraformStateNotProvisioned,
			expected: true,
		},
		{
			name:     "Wrapped ErrTerraformStateNotProvisioned is recoverable",
			err:      fmt.Errorf("component not found: %w", errUtils.ErrTerraformStateNotProvisioned),
			expected: true,
		},
		{
			name:     "ErrTerraformOutputNotFound is recoverable",
			err:      errUtils.ErrTerraformOutputNotFound,
			expected: true,
		},
		{
			name:     "Wrapped ErrTerraformOutputNotFound is recoverable",
			err:      fmt.Errorf("output missing: %w", errUtils.ErrTerraformOutputNotFound),
			expected: true,
		},
		{
			name:     "ErrGetObjectFromS3 is not recoverable",
			err:      errUtils.ErrGetObjectFromS3,
			expected: false,
		},
		{
			name:     "ErrTerraformBackendAPIError is not recoverable",
			err:      errUtils.ErrTerraformBackendAPIError,
			expected: false,
		},
		{
			name:     "Generic error is not recoverable",
			err:      errors.New("some random error"),
			expected: false,
		},
		{
			name:     "Nil error is not recoverable",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRecoverableTerraformError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsRecoverableInWarnMode pins which failures warn/silent mode degrades to the
// `(computed)` placeholder instead of aborting the command.
//
// Widened for https://github.com/cloudposse/atmos/issues/2566: a backend read that failed
// (ErrReadTerraformState — most importantly a cross-account `AccessDenied`) is now
// recoverable in warn mode. In a multi-account repository, a command that walks every stack
// will always meet backends the current identity cannot read; that is the topology working
// as designed, not a defect, and it must not abort an inventory listing. `--error-mode=strict`
// still surfaces all of these.
//
// The classifier keys on ErrReadTerraformState because that is the wrapper the YAML-function
// path actually produces: GetTerraformState wraps every backend failure in it (with the
// underlying cause formatted via %v, so inner sentinels like ErrGetObjectFromS3 are not in
// the chain). The bare-sentinel cases below document that boundary.
func TestIsRecoverableInWarnMode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "ErrTerraformStateNotProvisioned is recoverable",
			err:      errUtils.ErrTerraformStateNotProvisioned,
			expected: true,
		},
		{
			name:     "ErrTerraformOutputNotFound is recoverable",
			err:      errUtils.ErrTerraformOutputNotFound,
			expected: true,
		},
		{
			name:     "ErrReadTerraformState is recoverable in warn mode",
			err:      errUtils.ErrReadTerraformState,
			expected: true,
		},
		{
			name: "cross-account AccessDenied, as GetTerraformState actually wraps it, is recoverable",
			err: fmt.Errorf(
				"%w for component `global` in stack `dev-pen`: operation error S3: GetObject, "+
					"https response error StatusCode: 403, api error AccessDenied",
				errUtils.ErrReadTerraformState,
			),
			expected: true,
		},
		{
			name: "credential-refresh failure, as GetTerraformState actually wraps it, is recoverable",
			err: fmt.Errorf(
				"%w for component `vpc` in stack `dev`: get identity: get credentials: "+
					"failed to refresh cached credentials",
				errUtils.ErrReadTerraformState,
			),
			expected: true,
		},
		{
			name:     "bad YQ expression against retrieved state stays fatal",
			err:      errUtils.ErrEvaluateTerraformBackendVariable,
			expected: false,
		},
		{
			name:     "bare ErrGetObjectFromS3 is not recoverable: the path always wraps it in ErrReadTerraformState",
			err:      errUtils.ErrGetObjectFromS3,
			expected: false,
		},
		{
			name:     "bare ErrTerraformBackendAPIError is not recoverable for the same reason",
			err:      errUtils.ErrTerraformBackendAPIError,
			expected: false,
		},
		{
			name:     "Generic error is not recoverable",
			err:      errors.New("some random error"),
			expected: false,
		},
		{
			name:     "Nil error is not recoverable",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRecoverableInWarnMode(tt.err)
			assert.Equal(t, tt.expected, result)

			// The YQ `//` default operator must stay strictly narrower than warn mode: a
			// credential failure must never be papered over with a literal fallback value.
			if errors.Is(tt.err, errUtils.ErrReadTerraformState) {
				assert.False(t, isRecoverableTerraformError(tt.err),
					"a backend read failure must not enable the YQ default operator")
			}
		})
	}
}

//nolint:dupl // Test structure is similar to TestHasSchemeSeparator but tests completely different function (YQ expressions vs URI schemes).
func TestHasYqDefault(t *testing.T) {
	tests := []struct {
		name     string
		yqExpr   string
		expected bool
	}{
		{
			name:     "Expression with string default",
			yqExpr:   `.bucket_name // "default"`,
			expected: true,
		},
		{
			name:     "Expression with list default",
			yqExpr:   `.subnets // ["a", "b"]`,
			expected: true,
		},
		{
			name:     "Expression with map default",
			yqExpr:   `.tags // {"env": "dev"}`,
			expected: true,
		},
		{
			name:     "Expression with numeric default",
			yqExpr:   `.port // 8080`,
			expected: true,
		},
		{
			name:     "Expression with boolean default",
			yqExpr:   `.enabled // true`,
			expected: true,
		},
		{
			name:     "Expression with empty string default",
			yqExpr:   `.value // ""`,
			expected: true,
		},
		{
			name:     "Expression with null default",
			yqExpr:   `.value // null`,
			expected: true,
		},
		{
			name:     "Simple field access without default",
			yqExpr:   `.bucket_name`,
			expected: false,
		},
		{
			name:     "Array access without default",
			yqExpr:   `.subnets[0]`,
			expected: false,
		},
		{
			name:     "Nested field access without default",
			yqExpr:   `.config.database.host`,
			expected: false,
		},
		{
			name:     "Field name without dot",
			yqExpr:   `bucket_name`,
			expected: false,
		},
		{
			name:     "Empty string",
			yqExpr:   ``,
			expected: false,
		},
		{
			name:     "Expression with pipe but no default",
			yqExpr:   `.items | length`,
			expected: false,
		},
		{
			name:     "Expression with multiple defaults (chained)",
			yqExpr:   `.value // .fallback // "default"`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasYqDefault(tt.yqExpr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseTerraformStateArgs guards the invocation forms used by `!terraform.state`
// fixtures against the shared function argument parser, including the caller's
// empty-stack fallback to the current stack.
func TestParseTerraformStateArgs(t *testing.T) {
	tests := []struct {
		name           string
		args           string
		currentStack   string
		wantComponent  string
		wantStack      string
		wantExpression string
	}{
		{
			name:           "legacy component and output",
			args:           "kms-key key_arn",
			currentStack:   "plat-ue2-dev",
			wantComponent:  "kms-key",
			wantStack:      "plat-ue2-dev",
			wantExpression: "key_arn",
		},
		{
			name:           "legacy component stack and output",
			args:           "kms-key plat-ue2-prod key_arn",
			currentStack:   "plat-ue2-dev",
			wantComponent:  "kms-key",
			wantStack:      "plat-ue2-prod",
			wantExpression: "key_arn",
		},
		{
			name:           "single quoted expression with a default",
			args:           `kms-key '.key_arn // "mock-value"'`,
			currentStack:   "plat-ue2-dev",
			wantComponent:  "kms-key",
			wantStack:      "plat-ue2-dev",
			wantExpression: `.key_arn // "mock-value"`,
		},
		{
			name:           "whole YAML value quoted before parsing",
			args:           `kms-key .key_arn // "mock-value"`,
			currentStack:   "plat-ue2-dev",
			wantComponent:  "kms-key",
			wantStack:      "plat-ue2-dev",
			wantExpression: `.key_arn // "mock-value"`,
		},
		{
			name:           "stack with single quoted expression and a default",
			args:           `kms-key plat-ue2-prod '.key_arn // "mock-value"'`,
			currentStack:   "plat-ue2-dev",
			wantComponent:  "kms-key",
			wantStack:      "plat-ue2-prod",
			wantExpression: `.key_arn // "mock-value"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := fnparser.ParseTerraform(tt.args)

			assert.NoError(t, err)
			stack := parsed.Stack
			if stack == "" {
				stack = tt.currentStack
			}
			assert.Equal(t, tt.wantComponent, parsed.Component)
			assert.Equal(t, tt.wantStack, stack)
			assert.Equal(t, tt.wantExpression, parsed.Expression)
		})
	}
}

// TestTerraformState_YqDefaultWhenBackendReturnsNil verifies that YQ default
// values work when the backend returns nil (component not provisioned).
// The mock returns the ErrTerraformStateNotProvisioned sentinel error to simulate
// the real behavior when a component is not provisioned.
func TestTerraformState_LegacyCSVCompatibility(t *testing.T) {
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

	// Compatibility-only fixture: legacy CSV escaping remains supported at runtime.
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
		"bucket": `!terraform.state vpc test-stack .bucket_name // "default-bucket"`,
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
		"subnets": `!terraform.state vpc test-stack .subnets // ["subnet-1", "subnet-2"]`,
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

// TestTerraformState_OutputNotFoundWithDefaultUsesDefault verifies that when
// GetState returns ErrTerraformOutputNotFound (output key doesn't exist in state)
// AND the expression has a YQ default, the default is used.
func TestTerraformState_OutputNotFoundWithDefaultUsesDefault(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	expectedYqExpr := `.missing_output // "fallback-value"`

	// Mock returns ErrTerraformOutputNotFound - the output key doesn't exist.
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
		Return(nil, fmt.Errorf("output key not found: %w", errUtils.ErrTerraformOutputNotFound)).
		Times(1)

	input := schema.AtmosSectionMapType{
		"value": `!terraform.state vpc test-stack .missing_output // "fallback-value"`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	// With a YQ default and ErrTerraformOutputNotFound, the default should be used.
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "fallback-value", result["value"])
}

// TestTerraformState_YqDefaultWithMapFallback verifies that YQ default
// values work with map/object fallback expressions.
func TestTerraformState_YqDefaultWithMapFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	expectedYqExpr := `.tags // {"env": "dev", "team": "platform"}`

	// Mock returns ErrTerraformStateNotProvisioned - component not provisioned.
	mockStateGetter.EXPECT().
		GetState(
			atmosConfig,
			gomock.Any(),
			"test-stack",
			"config",
			expectedYqExpr,
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, fmt.Errorf("%w for component `config` in stack `test-stack`", errUtils.ErrTerraformStateNotProvisioned)).
		Times(1)

	input := schema.AtmosSectionMapType{
		"tags": `!terraform.state config test-stack .tags // {"env": "dev", "team": "platform"}`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Verify the map structure is returned correctly.
	expectedMap := map[string]any{"env": "dev", "team": "platform"}
	assert.Equal(t, expectedMap, result["tags"])
}

// TestTerraformState_YqDefaultWithNumericFallback verifies that
// numeric defaults work correctly.
func TestTerraformState_YqDefaultWithNumericFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	expectedYqExpr := `.replicas // 3`

	// Mock returns ErrTerraformStateNotProvisioned.
	mockStateGetter.EXPECT().
		GetState(
			atmosConfig,
			gomock.Any(),
			"test-stack",
			"app",
			expectedYqExpr,
			false,
			gomock.Any(),
			gomock.Any(),
		).
		Return(nil, fmt.Errorf("%w for component `app` in stack `test-stack`", errUtils.ErrTerraformStateNotProvisioned)).
		Times(1)

	input := schema.AtmosSectionMapType{
		"replicas": `!terraform.state app test-stack .replicas // 3`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Numeric default should work.
	assert.Equal(t, 3, result["replicas"])
}

// TestTerraformState_YqDefaultWithEmptyListFallback verifies that
// empty list defaults work correctly.
func TestTerraformState_YqDefaultWithEmptyListFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	expectedYqExpr := `.security_groups // []`

	// Mock returns ErrTerraformStateNotProvisioned.
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
		"security_groups": `!terraform.state vpc test-stack .security_groups // []`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Empty list default should work.
	assert.Equal(t, []any{}, result["security_groups"])
}
