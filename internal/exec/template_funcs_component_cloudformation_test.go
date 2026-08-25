package exec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

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
