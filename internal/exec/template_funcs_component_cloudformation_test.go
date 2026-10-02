package exec

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// clearComponentFuncSyncMap empties componentFuncSyncMap, restoring it after
// the test — componentFunc caches its result by stack/component, so tests
// must not leak cache entries into (or read stale ones from) other tests.
func clearComponentFuncSyncMap(t *testing.T) {
	t.Helper()
	clear := func() {
		componentFuncSyncMap.Range(func(key, _ any) bool {
			componentFuncSyncMap.Delete(key)
			return true
		})
	}
	clear()
	t.Cleanup(clear)
}

// componentFunc's aws/cloudformation branch must populate the result's
// `outputs` section from cloudFormationOutputsForSections (via the stubbed
// cloudFormationOutputsGetter seam), the CFN counterpart to the Terraform
// branch already covered by TestComponentFunc.
func TestComponentFunc_CloudFormationBranch_PopulatesOutputs(t *testing.T) {
	clearComponentFuncSyncMap(t)
	atmosConfig := setupAwsCloudFormationOutputFixture(t)

	ctrl := gomock.NewController(t)
	mockGetter := NewMockCloudFormationOutputsGetter(ctrl)
	mockGetter.EXPECT().GetOutputs(gomock.Any(), "us-east-1", "test-vpc", gomock.Any()).
		Return(map[string]any{"VpcId": "vpc-123"}, nil)
	stubCloudFormationOutputsGetter(t, mockGetter)

	result, err := componentFunc(&atmosConfig, nil, "vpc", "test")
	require.NoError(t, err)

	sections, ok := result.(map[string]any)
	require.True(t, ok, "componentFunc must return the sections map")
	outputs, ok := sections[cfg.OutputsSectionName].(map[string]any)
	require.True(t, ok, "sections must carry a populated outputs map")
	assert.Equal(t, "vpc-123", outputs["VpcId"])
}

// componentFunc's aws/cloudformation branch must propagate a
// cloudFormationOutputsForSections failure, such as a missing stack_name,
// wrapped with the atmos.Component context.
func TestComponentFunc_CloudFormationBranch_OutputsError(t *testing.T) {
	clearComponentFuncSyncMap(t)
	atmosConfig := setupAwsCloudFormationOutputFixture(t)
	stubCloudFormationOutputsGetter(t, nil) // any call would nil-panic, proving it's never reached.

	_, err := componentFunc(&atmosConfig, nil, "no-stack-name", "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "atmos.Component")
}

// atmos.Component(...).outputs shares the outputs getter, so a target stack
// that is not deployed must fail there too rather than yield an empty map.
func TestComponentFunc_CloudFormationBranch_NotDeployedStackFails(t *testing.T) {
	clearComponentFuncSyncMap(t)
	atmosConfig := setupAwsCloudFormationOutputFixture(t)

	ctrl := gomock.NewController(t)
	mockGetter := NewMockCloudFormationOutputsGetter(ctrl)
	notDeployed := fmt.Errorf("%w: %q has status %s", errUtils.ErrAwsCloudFormationStackNotDeployed, "test-vpc", "REVIEW_IN_PROGRESS")
	mockGetter.EXPECT().GetOutputs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, notDeployed)
	stubCloudFormationOutputsGetter(t, mockGetter)

	_, err := componentFunc(&atmosConfig, nil, "vpc", "test")
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationStackNotDeployed)
}

// A missing key is not an error for atmos.Component: the outputs map is
// returned as-is and Go templates handle absent keys themselves.
func TestComponentFunc_CloudFormationBranch_MissingKeyStillReturnsOutputs(t *testing.T) {
	clearComponentFuncSyncMap(t)
	atmosConfig := setupAwsCloudFormationOutputFixture(t)

	ctrl := gomock.NewController(t)
	mockGetter := NewMockCloudFormationOutputsGetter(ctrl)
	mockGetter.EXPECT().GetOutputs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(map[string]any{"VpcId": "vpc-123"}, nil)
	stubCloudFormationOutputsGetter(t, mockGetter)

	result, err := componentFunc(&atmosConfig, nil, "vpc", "test")
	require.NoError(t, err)
	outputs, ok := result.(map[string]any)[cfg.OutputsSectionName].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"VpcId": "vpc-123"}, outputs)
}

// componentFunc's aws/cloudformation branch must forward the resolved
// AuthManager's AuthContext (mirroring the Terraform branch) rather than
// always using a nil AuthContext.
func TestComponentFunc_CloudFormationBranch_PassesResolvedAuthContext(t *testing.T) {
	clearComponentFuncSyncMap(t)
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

	parentContext := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "enclosing-identity"}}
	_, err := componentFunc(&atmosConfig, &schema.ConfigAndStacksInfo{AuthContext: parentContext}, "vpc", "test")
	require.NoError(t, err)
	require.NotNil(t, gotAuth, "the enclosing AuthContext must reach cloudFormationOutputsForSections")
	assert.Equal(t, "enclosing-identity", gotAuth.Profile)
}

// componentFunc's cache must be keyed on the resolved identity, not just stack+component: two
// calls for the same aws/cloudformation target under two different enclosing identities (e.g.
// two callers with different --identity) must each hit the outputs getter and must never return
// the other identity's cached result.
func TestComponentFunc_CloudFormationBranch_CacheKeyIncludesIdentity(t *testing.T) {
	clearComponentFuncSyncMap(t)
	atmosConfig := setupAwsCloudFormationOutputFixture(t)

	ctrl := gomock.NewController(t)
	mockGetter := NewMockCloudFormationOutputsGetter(ctrl)
	mockGetter.EXPECT().GetOutputs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _, _ string, authCtx *schema.AWSAuthContext) (map[string]any, error) {
			return map[string]any{"VpcId": "vpc-" + authCtx.Profile}, nil
		},
	).Times(2)
	stubCloudFormationOutputsGetter(t, mockGetter)

	identityA := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "identity-a"}}
	resultA, err := componentFunc(&atmosConfig, &schema.ConfigAndStacksInfo{AuthContext: identityA}, "vpc", "test")
	require.NoError(t, err)
	outputsA := resultA.(map[string]any)[cfg.OutputsSectionName].(map[string]any)
	assert.Equal(t, "vpc-identity-a", outputsA["VpcId"])

	identityB := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "identity-b"}}
	resultB, err := componentFunc(&atmosConfig, &schema.ConfigAndStacksInfo{AuthContext: identityB}, "vpc", "test")
	require.NoError(t, err)
	outputsB := resultB.(map[string]any)[cfg.OutputsSectionName].(map[string]any)
	assert.Equal(t, "vpc-identity-b", outputsB["VpcId"])
}

// Repeated eager references reuse only their own identity's cached outputs.
func TestComponentFunc_CloudFormationBranch_ReusesIdentityCache(t *testing.T) {
	clearComponentFuncSyncMap(t)
	ac := setupAwsCloudFormationOutputFixture(t)
	getter := NewMockCloudFormationOutputsGetter(gomock.NewController(t))
	stubCloudFormationOutputsGetter(t, getter)
	for _, profile := range []string{"first", "second"} {
		getter.EXPECT().GetOutputs(gomock.Any(), "us-east-1", "test-vpc", &schema.AWSAuthContext{Profile: profile}).
			Return(map[string]any{"VpcId": profile}, nil).Times(1)
	}
	for _, profile := range []string{"first", "second", "first", "second"} {
		info := &schema.ConfigAndStacksInfo{AuthContext: &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: profile}}}
		value, err := componentFunc(&ac, info, "vpc", "test")
		require.NoError(t, err)
		require.Equal(t, profile, value.(map[string]any)[cfg.OutputsSectionName].(map[string]any)["VpcId"])
	}
}

// Cached values remain usable when there are no outputs or diagnostic YAML
// rendering fails; neither case should trigger a fresh provider request.
func TestComponentFunc_CacheLoggingPreservesResults(t *testing.T) {
	for name, sections := range map[string]map[string]any{
		"no outputs":           {cfg.VarsSectionName: map[string]any{"id": "cached"}},
		"unrenderable outputs": {cfg.OutputsSectionName: yamlMarshalError{}},
	} {
		t.Run(name, func(t *testing.T) {
			clearComponentFuncSyncMap(t)
			ac := setupAwsCloudFormationOutputFixture(t)
			stubCloudFormationOutputsGetter(t, NewMockCloudFormationOutputsGetter(gomock.NewController(t)))
			componentFuncSyncMap.Store("test-vpc-", sections)
			value, err := componentFunc(&ac, nil, "vpc", "test")
			require.NoError(t, err)
			require.Equal(t, sections, value)
		})
	}
}
