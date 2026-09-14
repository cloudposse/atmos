package cloudformation

import (
	"testing"

	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestBuildStackSpec_RequiresStackName(t *testing.T) {
	_, err := buildStackSpec(map[string]any{"template": "template.yaml"})
	require.ErrorIs(t, err, errUtils.ErrMissingAwsCloudFormationStackName)
}

func TestBuildStackSpec_RequiresTemplateUnlessAbstract(t *testing.T) {
	_, err := buildStackSpec(map[string]any{"stack_name": "vpc"})
	require.ErrorIs(t, err, errUtils.ErrMissingAwsCloudFormationTemplate)

	spec, err := buildStackSpec(map[string]any{
		"stack_name": "vpc",
		"metadata":   map[string]any{"type": "abstract"},
	})
	require.NoError(t, err)
	assert.Equal(t, "vpc", spec.StackName)
	assert.Empty(t, spec.TemplatePath)
}

func TestBuildStackSpec_FullConfig(t *testing.T) {
	componentSection := map[string]any{
		"stack_name": "acme-plat-ue2-dev-vpc",
		"template":   "template.yaml",
		"parameters": map[string]any{
			"CidrBlock":   "10.0.0.0/16",
			"Environment": "dev",
			"AZs":         []any{"us-east-2a", "us-east-2b"},
		},
		"capabilities": []any{"CAPABILITY_IAM", "CAPABILITY_NAMED_IAM"},
		"tags":         map[string]any{"Team": "platform", "Environment": "dev"},
		"stack_policy": map[string]any{"file": "stack-policy.json"},
		"role_arn":     "arn:aws:iam::123456789012:role/cfn-deploy",
		"notification_arns": []any{
			"arn:aws:sns:us-east-2:123456789012:notify",
		},
		"disable_rollback":       true,
		"termination_protection": true,
		"timeout_in_minutes":     30,
	}

	spec, err := buildStackSpec(componentSection)
	require.NoError(t, err)

	assert.Equal(t, "acme-plat-ue2-dev-vpc", spec.StackName)
	assert.Equal(t, "template.yaml", spec.TemplatePath)
	assert.Equal(t, "stack-policy.json", spec.StackPolicyFile)
	assert.Equal(t, "arn:aws:iam::123456789012:role/cfn-deploy", spec.RoleArn)
	assert.Equal(t, []string{"arn:aws:sns:us-east-2:123456789012:notify"}, spec.NotificationArns)
	assert.True(t, spec.DisableRollback)
	assert.True(t, spec.TerminationProtection)
	assert.Equal(t, int32(30), spec.TimeoutInMinutes)

	require.Len(t, spec.Parameters, 3)
	paramMap := make(map[string]string, len(spec.Parameters))
	for _, p := range spec.Parameters {
		paramMap[*p.ParameterKey] = *p.ParameterValue
	}
	assert.Equal(t, "10.0.0.0/16", paramMap["CidrBlock"])
	assert.Equal(t, "dev", paramMap["Environment"])
	assert.Equal(t, "us-east-2a,us-east-2b", paramMap["AZs"], "list parameters are comma-joined for List<Type>/CommaDelimitedList")

	require.Len(t, spec.Capabilities, 2)
	assert.Contains(t, spec.Capabilities, cfntypes.Capability("CAPABILITY_IAM"))
	assert.Contains(t, spec.Capabilities, cfntypes.Capability("CAPABILITY_NAMED_IAM"))

	require.Len(t, spec.Tags, 2)
	tagMap := make(map[string]string, len(spec.Tags))
	for _, tag := range spec.Tags {
		tagMap[*tag.Key] = *tag.Value
	}
	assert.Equal(t, "platform", tagMap["Team"])
	assert.Equal(t, "dev", tagMap["Environment"])
}

func TestNormalizeParameters_RejectsNestedMap(t *testing.T) {
	_, err := normalizeParameters(map[string]any{
		"Bad": map[string]any{"nested": "value"},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
}

func TestNormalizeParameters_NilAndNonMap(t *testing.T) {
	params, err := normalizeParameters(nil)
	require.NoError(t, err)
	assert.Nil(t, params)

	params, err = normalizeParameters("not-a-map")
	require.NoError(t, err)
	assert.Nil(t, params)
}

// TestBuildStackSpec_PropagatesParameterError verifies buildStackSpec's own
// error-propagation branch for normalizeParameters failing, not just
// normalizeParameters itself (TestNormalizeParameters_RejectsNestedMap).
func TestBuildStackSpec_PropagatesParameterError(t *testing.T) {
	_, err := buildStackSpec(map[string]any{
		"stack_name": "vpc",
		"template":   "template.yaml",
		"parameters": map[string]any{
			"Bad": map[string]any{"nested": "value"},
		},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
}

// TestStringifyParameterValue covers every branch of stringifyParameterValue:
// string passthrough, list join, nested-list error propagation, nil, and the
// default scalar (%v) fallback for types like bool/int that aren't string.
func TestStringifyParameterValue(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		v, err := stringifyParameterValue("hello")
		require.NoError(t, err)
		assert.Equal(t, "hello", v)
	})

	t.Run("list is comma-joined", func(t *testing.T) {
		v, err := stringifyParameterValue([]any{"a", "b", "c"})
		require.NoError(t, err)
		assert.Equal(t, "a,b,c", v)
	})

	t.Run("list propagates a nested item's error", func(t *testing.T) {
		_, err := stringifyParameterValue([]any{"ok", map[string]any{"nested": "value"}})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
	})

	t.Run("nil becomes empty string", func(t *testing.T) {
		v, err := stringifyParameterValue(nil)
		require.NoError(t, err)
		assert.Equal(t, "", v)
	})

	t.Run("bool falls back to default %v formatting", func(t *testing.T) {
		v, err := stringifyParameterValue(true)
		require.NoError(t, err)
		assert.Equal(t, "true", v)
	})

	t.Run("map is rejected", func(t *testing.T) {
		_, err := stringifyParameterValue(map[string]any{"x": "y"})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidAwsCloudFormationSettings)
	})
}

// TestNormalizeStringSlice_AlreadyStringSlice covers the []string passthrough
// branch (as opposed to the []any conversion branch already exercised by
// TestBuildStackSpec_FullConfig/TestNormalizeCapabilities_Empty).
func TestNormalizeStringSlice_AlreadyStringSlice(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, normalizeStringSlice([]string{"a", "b"}))
}

func TestNormalizeCapabilities_Empty(t *testing.T) {
	assert.Nil(t, normalizeCapabilities(nil))
	assert.Nil(t, normalizeCapabilities([]any{}))
}

func TestNormalizeTags_NonMap(t *testing.T) {
	assert.Nil(t, normalizeTags("not-a-map"))
}

func TestToInt32_ClampsOverflow(t *testing.T) {
	assert.Equal(t, int32(30), toInt32(30))
	assert.Equal(t, int32(30), toInt32(int32(30)))
	assert.Equal(t, int32(30), toInt32(int64(30)))
	assert.Equal(t, int32(30), toInt32(float64(30)))
	assert.Equal(t, int32(2147483647), toInt32(int64(9999999999)))
	assert.Equal(t, int32(-2147483648), toInt32(int64(-9999999999)))
	assert.Equal(t, int32(0), toInt32("not-a-number"))
}

func TestIsAbstractComponent(t *testing.T) {
	assert.True(t, isAbstractComponent(map[string]any{
		"metadata": map[string]any{"type": "abstract"},
	}))
	assert.False(t, isAbstractComponent(map[string]any{
		"metadata": map[string]any{"type": "real"},
	}))
	assert.False(t, isAbstractComponent(map[string]any{}))
}
