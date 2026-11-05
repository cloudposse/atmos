package backend

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDeleteCmd_Structure(t *testing.T) {
	testCommandStructure(t, &commandTestParams{
		cmd:              deleteCmd,
		parser:           deleteParser,
		expectedUse:      "delete [component]",
		expectedShort:    "Delete the template-packaging backend bucket",
		requiredFlags:    []string{"force", "target"},
		hasPositionalArg: true,
	})

	t.Run("force flag is boolean", func(t *testing.T) {
		forceFlag := deleteCmd.Flags().Lookup("force")
		assert.NotNil(t, forceFlag, "force flag should be registered")
		assert.Equal(t, "bool", forceFlag.Value.Type())
	})
}

func TestDeleteCmd_Init(t *testing.T) {
	assert.NotNil(t, deleteParser, "deleteParser should be initialized")
	assert.NotNil(t, deleteCmd, "deleteCmd should be initialized")
	assert.False(t, deleteCmd.DisableFlagParsing, "DisableFlagParsing should be false")

	forceFlag := deleteCmd.Flags().Lookup("force")
	assert.NotNil(t, forceFlag, "force flag should be registered")
}

func TestExecuteDelete_RequiresStack(t *testing.T) {
	err := executeDelete(t.Context(), deleteRequest{Component: "vpc", Stack: "", Force: true})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

func TestExecuteDelete_ConfigInitError(t *testing.T) {
	t.Cleanup(ResetDependencies)

	ctrl := gomock.NewController(t)
	mockCI := NewMockConfigInitializer(ctrl)
	mockCI.EXPECT().InitConfigAndAuth("vpc", "dev", "").Return(nil, nil, errors.New("boom"))
	SetConfigInitializer(mockCI)

	err := executeDelete(t.Context(), deleteRequest{Component: "vpc", Stack: "dev", Force: true})
	require.Error(t, err)
}

func TestExecuteDelete_Success(t *testing.T) {
	t.Cleanup(ResetDependencies)

	ctrl := gomock.NewController(t)
	mockCI := NewMockConfigInitializer(ctrl)
	mockProv := NewMockProvisioner(ctrl)

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{}
	componentConfig := singleS3TargetComponentConfig()

	mockCI.EXPECT().InitConfigAndAuth("vpc", "dev", "").Return(atmosConfig, info, nil)
	mockCI.EXPECT().DescribeComponent(atmosConfig, info, "vpc", "dev").Return(componentConfig, nil)
	mockProv.EXPECT().DeleteBackend(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, params *DeleteBackendParams) error {
		assert.True(t, params.Force)
		assert.Equal(t, componentConfig, params.ComponentConfig)
		return nil
	})

	SetConfigInitializer(mockCI)
	SetProvisioner(mockProv)

	err := executeDelete(t.Context(), deleteRequest{Component: "vpc", Stack: "dev", Force: true})
	require.NoError(t, err)
}
