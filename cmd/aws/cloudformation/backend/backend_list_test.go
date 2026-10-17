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

func TestListCmd_Structure(t *testing.T) {
	testCommandStructure(t, &commandTestParams{
		cmd:              listCmd,
		parser:           listParser,
		expectedUse:      "list [component]",
		expectedShort:    "List the component's template-packaging backend targets",
		requiredFlags:    []string{"format"},
		hasPositionalArg: true,
	})
}

func TestListCmd_Init(t *testing.T) {
	assert.NotNil(t, listParser, "listParser should be initialized")
	assert.NotNil(t, listCmd, "listCmd should be initialized")

	formatFlag := listCmd.Flags().Lookup("format")
	assert.NotNil(t, formatFlag, "format flag should be registered")
	assert.Equal(t, "table", formatFlag.DefValue)
}

func TestExecuteList_RequiresStack(t *testing.T) {
	err := executeList(t.Context(), "vpc", "", "", "table")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

func TestExecuteList_ConfigInitError(t *testing.T) {
	t.Cleanup(ResetDependencies)

	ctrl := gomock.NewController(t)
	mockCI := NewMockConfigInitializer(ctrl)
	mockCI.EXPECT().InitConfigAndAuth("vpc", "dev", "").Return(nil, nil, errors.New("boom"))
	SetConfigInitializer(mockCI)

	err := executeList(t.Context(), "vpc", "dev", "", "table")
	require.Error(t, err)
}

func TestExecuteList_Success(t *testing.T) {
	t.Cleanup(ResetDependencies)

	ctrl := gomock.NewController(t)
	mockCI := NewMockConfigInitializer(ctrl)
	mockProv := NewMockProvisioner(ctrl)

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{}
	componentConfig := singleS3TargetComponentConfig()

	mockCI.EXPECT().InitConfigAndAuth("vpc", "dev", "").Return(atmosConfig, info, nil)
	mockCI.EXPECT().DescribeComponent(atmosConfig, info, "vpc", "dev").Return(componentConfig, nil)
	mockProv.EXPECT().ListBackends(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, params *ListBackendsParams) error {
		assert.Equal(t, "vpc", params.Component)
		assert.Equal(t, componentConfig, params.ComponentConfig)
		return nil
	})

	SetConfigInitializer(mockCI)
	SetProvisioner(mockProv)

	err := executeList(t.Context(), "vpc", "dev", "", "table")
	require.NoError(t, err)
}
