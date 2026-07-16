package exec

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestProcessCustomYamlTagsLenient_RecoverableError_Strict verifies that
// ProcessCustomYamlTags (the strict entry point) still fails outright on a recoverable
// error with no onWarning callback, i.e. lenient behavior is opt-in only.
func TestProcessCustomYamlTagsLenient_RecoverableError_Strict(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}

	mockStateGetter.EXPECT().
		GetState(atmosConfig, gomock.Any(), "test-stack", "vpc", "bucket_name", false, gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("%w for component `vpc` in stack `test-stack`", errUtils.ErrTerraformStateNotProvisioned)).
		Times(1)

	input := schema.AtmosSectionMapType{
		"bucket": `!terraform.state vpc test-stack bucket_name`,
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTerraformStateNotProvisioned)
	assert.Nil(t, result)
}

// TestProcessCustomYamlTagsLenient_RecoverableError_Warn verifies that in lenient (warn)
// mode, a recoverable error is substituted with degradation.AtmosComputedValue{}, reported
// via onWarning exactly once with the right stack/component/function/reason, and sibling
// keys still resolve.
func TestProcessCustomYamlTagsLenient_RecoverableError_Warn(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}

	recoverableErr := fmt.Errorf("%w for component `vpc` in stack `test-stack`", errUtils.ErrTerraformStateNotProvisioned)
	mockStateGetter.EXPECT().
		GetState(atmosConfig, gomock.Any(), "test-stack", "vpc", "bucket_name", false, gomock.Any(), gomock.Any()).
		Return(nil, recoverableErr).
		Times(1)

	input := schema.AtmosSectionMapType{
		"bucket":  `!terraform.state vpc test-stack bucket_name`,
		"sibling": "unaffected-value",
	}

	stackInfo := &schema.ConfigAndStacksInfo{Component: "vpc", Stack: "test-stack"}

	var warnings []DegradationWarning
	result, err := ProcessCustomYamlTagsLenient(atmosConfig, input, "test-stack", nil, stackInfo, func(w DegradationWarning) {
		warnings = append(warnings, w)
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, degradation.AtmosComputedValue{}, result["bucket"])
	assert.Equal(t, "unaffected-value", result["sibling"])

	require.Len(t, warnings, 1)
	assert.Equal(t, "test-stack", warnings[0].Stack)
	assert.Equal(t, "vpc", warnings[0].Component)
	assert.Contains(t, warnings[0].Function, "!terraform.state")
	assert.Contains(t, warnings[0].Reason, "terraform state not provisioned")
}

// TestProcessCustomYamlTagsLenient_RecoverableError_NilCallback verifies that the
// lenient entry point remains lenient when the caller does not need warning details.
func TestProcessCustomYamlTagsLenient_RecoverableError_NilCallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	mockStateGetter.EXPECT().
		GetState(atmosConfig, gomock.Any(), "test-stack", "vpc", "bucket_name", false, gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("%w for component `vpc` in stack `test-stack`", errUtils.ErrTerraformStateNotProvisioned))

	result, err := ProcessCustomYamlTagsLenient(
		atmosConfig,
		schema.AtmosSectionMapType{"bucket": `!terraform.state vpc test-stack bucket_name`},
		"test-stack", nil, &schema.ConfigAndStacksInfo{Component: "vpc", Stack: "test-stack"}, nil,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, degradation.AtmosComputedValue{}, result["bucket"])
}

// TestProcessCustomYamlTagsLenient_NonRecoverableError_StillFails verifies that lenient
// mode does not blanket-catch every error: a non-recoverable error (e.g. a real API
// failure) still aborts the call, proving the classifier gate rather than a blind catch.
func TestProcessCustomYamlTagsLenient_NonRecoverableError_StillFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}

	mockStateGetter.EXPECT().
		GetState(atmosConfig, gomock.Any(), "test-stack", "vpc", "bucket_name", false, gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("S3 GetObject timeout after 30s")).
		Times(1)

	input := schema.AtmosSectionMapType{
		"bucket": `!terraform.state vpc test-stack bucket_name`,
	}

	stackInfo := &schema.ConfigAndStacksInfo{Component: "vpc", Stack: "test-stack"}

	var warnings []DegradationWarning
	result, err := ProcessCustomYamlTagsLenient(atmosConfig, input, "test-stack", nil, stackInfo, func(w DegradationWarning) {
		warnings = append(warnings, w)
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "S3 GetObject timeout")
	assert.Empty(t, warnings, "non-recoverable errors must not trigger onWarning")
}
