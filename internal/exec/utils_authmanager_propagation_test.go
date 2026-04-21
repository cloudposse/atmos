package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProcessComponentConfig_PropagatesAuthManager(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAuthManager := types.NewMockAuthManager(ctrl)
	expectedAuthContext := &schema.AuthContext{
		Identity: "terraform",
	}
	mockAuthManager.EXPECT().
		GetStackInfo().
		Return(&schema.ConfigAndStacksInfo{AuthContext: expectedAuthContext}).
		Times(1)

	stacksMap := map[string]any{
		"tenant-dev-test": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{
							"name": "vpc",
						},
					},
				},
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{}
	err := ProcessComponentConfig(
		&schema.AtmosConfiguration{},
		info,
		"tenant-dev-test",
		stacksMap,
		"terraform",
		"vpc",
		mockAuthManager,
	)
	require.NoError(t, err)
	assert.Equal(t, mockAuthManager, info.AuthManager)
	assert.Equal(t, expectedAuthContext, info.AuthContext)
}
