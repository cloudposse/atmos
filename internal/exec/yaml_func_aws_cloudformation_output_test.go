package exec

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// parseAwsCloudFormationOutputArgs must parse the `component [stack]
// output-key` grammar, defaulting Stack to currentStack when omitted, and
// must propagate a malformed-input error.
func TestParseAwsCloudFormationOutputArgs(t *testing.T) {
	t.Run("stack omitted defaults to currentStack", func(t *testing.T) {
		parsed, err := parseAwsCloudFormationOutputArgs("!aws.cloudformation.output vpc VpcId", "current-stack")
		require.NoError(t, err)
		assert.Equal(t, "vpc", parsed.Component)
		assert.Equal(t, "current-stack", parsed.Stack)
		assert.Equal(t, "VpcId", parsed.Expression)
	})

	t.Run("explicit stack overrides currentStack", func(t *testing.T) {
		parsed, err := parseAwsCloudFormationOutputArgs("!aws.cloudformation.output vpc other-stack VpcId", "current-stack")
		require.NoError(t, err)
		assert.Equal(t, "vpc", parsed.Component)
		assert.Equal(t, "other-stack", parsed.Stack)
		assert.Equal(t, "VpcId", parsed.Expression)
	})

	t.Run("empty arguments propagate the getStringAfterTag error", func(t *testing.T) {
		_, err := parseAwsCloudFormationOutputArgs("!aws.cloudformation.output", "current-stack")
		require.Error(t, err)
	})

	t.Run("a single argument propagates a ParseTerraform error (2 or 3 arguments required)", func(t *testing.T) {
		_, err := parseAwsCloudFormationOutputArgs("!aws.cloudformation.output vpc", "current-stack")
		require.Error(t, err)
	})
}

// setupAwsCloudFormationOutputFixture chdirs into the shared aws/cloudformation
// output fixture (tests/fixtures/scenarios/aws-cloudformation-outputs) and
// returns its loaded AtmosConfiguration.
func setupAwsCloudFormationOutputFixture(t *testing.T) schema.AtmosConfiguration {
	t.Helper()
	t.Chdir("../../tests/fixtures/scenarios/aws-cloudformation-outputs")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	return atmosConfig
}

// processTagAwsCloudFormationOutputWithContext must propagate a
// parseAwsCloudFormationOutputArgs failure (malformed input) without ever
// reaching trackOutputDependency, describe, or the outputs getter.
func TestProcessTagAwsCloudFormationOutputWithContext_ParseArgsError(t *testing.T) {
	atmosConfig := setupAwsCloudFormationOutputFixture(t)
	stubCloudFormationOutputsGetter(t, nil) // any call would nil-panic, proving it's never reached.

	_, err := processTagAwsCloudFormationOutputWithContext(
		&atmosConfig, "!aws.cloudformation.output", "test", nil, nil,
	)
	require.Error(t, err)
}

// processTagAwsCloudFormationOutputWithContext's happy path: resolves the
// target's stack_name/region from its described sections and returns the
// requested output's value from the (stubbed) outputs getter.
func TestProcessTagAwsCloudFormationOutputWithContext_Success(t *testing.T) {
	atmosConfig := setupAwsCloudFormationOutputFixture(t)

	ctrl := gomock.NewController(t)
	mockGetter := NewMockCloudFormationOutputsGetter(ctrl)
	mockGetter.EXPECT().GetOutputs(gomock.Any(), "us-east-1", "test-vpc", gomock.Any()).
		Return(map[string]any{"VpcId": "vpc-123"}, nil)
	stubCloudFormationOutputsGetter(t, mockGetter)

	result, err := processTagAwsCloudFormationOutputWithContext(
		&atmosConfig, "!aws.cloudformation.output vpc test VpcId", "test", nil, nil,
	)
	require.NoError(t, err)
	assert.Equal(t, "vpc-123", result)
}

// A requested output key absent from the deployed stack's Outputs must
// return (nil, nil) — not an error — matching !terraform.output's own
// "output not found" contract.
func TestProcessTagAwsCloudFormationOutputWithContext_OutputKeyNotFound(t *testing.T) {
	atmosConfig := setupAwsCloudFormationOutputFixture(t)

	ctrl := gomock.NewController(t)
	mockGetter := NewMockCloudFormationOutputsGetter(ctrl)
	mockGetter.EXPECT().GetOutputs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[string]any{"OtherKey": "other-value"}, nil)
	stubCloudFormationOutputsGetter(t, mockGetter)

	result, err := processTagAwsCloudFormationOutputWithContext(
		&atmosConfig, "!aws.cloudformation.output vpc test MissingKey", "test", nil, nil,
	)
	require.NoError(t, err)
	assert.Nil(t, result)
}

// A component with no stack_name must surface
// ErrMissingAwsCloudFormationStackName without ever reaching the outputs
// getter.
func TestProcessTagAwsCloudFormationOutputWithContext_MissingStackName(t *testing.T) {
	atmosConfig := setupAwsCloudFormationOutputFixture(t)
	stubCloudFormationOutputsGetter(t, nil) // any call would nil-panic, proving it's never reached.

	_, err := processTagAwsCloudFormationOutputWithContext(
		&atmosConfig, "!aws.cloudformation.output no-stack-name test SomeKey", "test", nil, nil,
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMissingAwsCloudFormationStackName)
}

// A target component that doesn't exist in the stack must surface a
// describe-component error.
func TestProcessTagAwsCloudFormationOutputWithContext_DescribeComponentError(t *testing.T) {
	atmosConfig := setupAwsCloudFormationOutputFixture(t)
	stubCloudFormationOutputsGetter(t, nil) // any call would nil-panic, proving it's never reached.

	_, err := processTagAwsCloudFormationOutputWithContext(
		&atmosConfig, "!aws.cloudformation.output does-not-exist test SomeKey", "test", nil, nil,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
}

// processTagAwsCloudFormationOutputWithContext must extract authContext and
// authManager from a populated stackInfo and thread the resolved AWS auth
// context through to the outputs getter (via resolveNestedOutputAuth and
// cloudFormationOutputsForSections), not just handle the nil-stackInfo case.
func TestProcessTagAwsCloudFormationOutputWithContext_PopulatedStackInfo(t *testing.T) {
	atmosConfig := setupAwsCloudFormationOutputFixture(t)

	ctrl := gomock.NewController(t)
	mockGetter := NewMockCloudFormationOutputsGetter(ctrl)
	var gotAuth *schema.AWSAuthContext
	mockGetter.EXPECT().GetOutputs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, authCtx *schema.AWSAuthContext) (map[string]any, error) {
			gotAuth = authCtx
			return map[string]any{"VpcId": "vpc-123"}, nil
		},
	)
	stubCloudFormationOutputsGetter(t, mockGetter)

	awsAuth := &schema.AWSAuthContext{Profile: "dev"}
	stackInfo := &schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{AWS: awsAuth}}

	result, err := processTagAwsCloudFormationOutputWithContext(
		&atmosConfig, "!aws.cloudformation.output vpc test VpcId", "test", nil, stackInfo,
	)
	require.NoError(t, err)
	assert.Equal(t, "vpc-123", result)
	require.NotNil(t, gotAuth, "the enclosing stackInfo's AWS auth context must reach the outputs getter")
	assert.Equal(t, "dev", gotAuth.Profile)
}

// A CloudFormationOutputsGetter error must propagate, wrapped with the
// component/stack/output context.
func TestProcessTagAwsCloudFormationOutputWithContext_GetterError(t *testing.T) {
	atmosConfig := setupAwsCloudFormationOutputFixture(t)

	ctrl := gomock.NewController(t)
	mockGetter := NewMockCloudFormationOutputsGetter(ctrl)
	sentinel := errors.New("stack not found")
	mockGetter.EXPECT().GetOutputs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, sentinel)
	stubCloudFormationOutputsGetter(t, mockGetter)

	_, err := processTagAwsCloudFormationOutputWithContext(
		&atmosConfig, "!aws.cloudformation.output vpc test VpcId", "test", nil, nil,
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

// processTagAwsCloudFormationOutputWithContext must actually push onto the
// given ResolutionContext (via trackOutputDependency) and surface
// ErrCircularDependency when the target component/stack pair is already on
// the call stack — proving cycle detection is wired in, not merely present
// in a shared helper that's never reached from this code path.
func TestProcessTagAwsCloudFormationOutputWithContext_CycleDetection(t *testing.T) {
	atmosConfig := setupAwsCloudFormationOutputFixture(t)
	stubCloudFormationOutputsGetter(t, nil) // any call would nil-panic, proving it's never reached.

	resolutionCtx := NewResolutionContext()
	require.NoError(t, resolutionCtx.Push(&atmosConfig, DependencyNode{
		Component:    "vpc",
		Stack:        "test",
		FunctionType: "terraform.output",
		FunctionCall: "!aws.cloudformation.output vpc test VpcId",
	}))
	t.Cleanup(func() { resolutionCtx.Pop(&atmosConfig) })

	_, err := processTagAwsCloudFormationOutputWithContext(
		&atmosConfig, "!aws.cloudformation.output vpc test VpcId", "test", resolutionCtx, nil,
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCircularDependency)
}

// A valid resolutionCtx must still push/pop cleanly through a successful
// call, leaving the stack empty afterward — the complement of the cycle test
// above (context is used correctly, not merely present).
func TestProcessTagAwsCloudFormationOutputWithContext_ResolutionContextCleansUpOnSuccess(t *testing.T) {
	atmosConfig := setupAwsCloudFormationOutputFixture(t)

	ctrl := gomock.NewController(t)
	mockGetter := NewMockCloudFormationOutputsGetter(ctrl)
	mockGetter.EXPECT().GetOutputs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[string]any{"VpcId": "vpc-123"}, nil)
	stubCloudFormationOutputsGetter(t, mockGetter)

	resolutionCtx := NewResolutionContext()
	_, err := processTagAwsCloudFormationOutputWithContext(
		&atmosConfig, "!aws.cloudformation.output vpc test VpcId", "test", resolutionCtx, nil,
	)
	require.NoError(t, err)
	assert.Empty(t, resolutionCtx.CallStack, "the pushed node must be popped after a successful resolution")
	assert.Empty(t, resolutionCtx.Visited)
}

// context.Background usage sanity: the getter must actually be invoked with
// a non-nil context (guards against a future refactor accidentally passing
// a nil context.Context to the AWS SDK layer).
func TestProcessTagAwsCloudFormationOutputWithContext_PassesNonNilContext(t *testing.T) {
	atmosConfig := setupAwsCloudFormationOutputFixture(t)

	ctrl := gomock.NewController(t)
	mockGetter := NewMockCloudFormationOutputsGetter(ctrl)
	var gotCtx context.Context
	mockGetter.EXPECT().GetOutputs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _, _ string, _ *schema.AWSAuthContext) (map[string]any, error) {
			gotCtx = ctx
			return map[string]any{"VpcId": "vpc-123"}, nil
		},
	)
	stubCloudFormationOutputsGetter(t, mockGetter)

	_, err := processTagAwsCloudFormationOutputWithContext(
		&atmosConfig, "!aws.cloudformation.output vpc test VpcId", "test", nil, nil,
	)
	require.NoError(t, err)
	require.NotNil(t, gotCtx)
}
