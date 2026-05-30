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

	mockAuthManager := types.NewMockAuthManager(ctrl)
	// AuthContext is a container of cloud-provider-specific credential records; the
	// concrete values do not matter for this propagation test, only that the same
	// pointer arrives in info.AuthContext.
	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{Profile: "terraform"},
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

func TestProcessComponentConfig_AuthManagerGuardBranches(t *testing.T) {
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

	t.Run("nil auth manager leaves auth fields unset", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{}
		err := ProcessComponentConfig(
			&schema.AtmosConfiguration{},
			info,
			"tenant-dev-test",
			stacksMap,
			"terraform",
			"vpc",
			nil,
		)
		require.NoError(t, err)
		assert.Nil(t, info.AuthManager)
		assert.Nil(t, info.AuthContext)
	})

	t.Run("nil stack info keeps manager and leaves auth context unset", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockAuthManager := types.NewMockAuthManager(ctrl)
		mockAuthManager.EXPECT().GetStackInfo().Return(nil).Times(1)

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
		assert.Nil(t, info.AuthContext)
	})

	t.Run("nil auth context keeps manager and leaves auth context unset", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockAuthManager := types.NewMockAuthManager(ctrl)
		mockAuthManager.EXPECT().
			GetStackInfo().
			Return(&schema.ConfigAndStacksInfo{AuthContext: nil}).
			Times(1)

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
		assert.Nil(t, info.AuthContext)
	})
}
