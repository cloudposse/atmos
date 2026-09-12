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

func TestDescribeCmd_Structure(t *testing.T) {
	testCommandStructure(t, &commandTestParams{
		cmd:              describeCmd,
		parser:           describeParser,
		expectedUse:      "describe [component]",
		expectedShort:    "Describe the template-packaging backend bucket",
		requiredFlags:    []string{"format", "target"},
		hasPositionalArg: true,
	})
}

func TestDescribeCmd_Init(t *testing.T) {
	assert.NotNil(t, describeParser, "describeParser should be initialized")
	assert.NotNil(t, describeCmd, "describeCmd should be initialized")

	formatFlag := describeCmd.Flags().Lookup("format")
	assert.NotNil(t, formatFlag, "format flag should be registered")
	assert.Equal(t, "table", formatFlag.DefValue)
}

func TestExecuteDescribe_RequiresStack(t *testing.T) {
	err := executeDescribe(t.Context(), &describeRequest{Component: "vpc", Stack: "", Format: "table"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

func TestExecuteDescribe_ConfigInitError(t *testing.T) {
	t.Cleanup(ResetDependencies)

	ctrl := gomock.NewController(t)
	mockCI := NewMockConfigInitializer(ctrl)
	mockCI.EXPECT().InitConfigAndAuth("vpc", "dev", "").Return(nil, nil, errors.New("boom"))
	SetConfigInitializer(mockCI)

	err := executeDescribe(t.Context(), &describeRequest{Component: "vpc", Stack: "dev", Format: "table"})
	require.Error(t, err)
}

func TestExecuteDescribe_Success(t *testing.T) {
	t.Cleanup(ResetDependencies)

	ctrl := gomock.NewController(t)
	mockCI := NewMockConfigInitializer(ctrl)
	mockProv := NewMockProvisioner(ctrl)

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{}
	componentConfig := singleS3TargetComponentConfig()

	mockCI.EXPECT().InitConfigAndAuth("vpc", "dev", "").Return(atmosConfig, info, nil)
	mockCI.EXPECT().DescribeComponent(atmosConfig, info, "vpc", "dev").Return(componentConfig, nil)
	mockProv.EXPECT().DescribeBackend(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, params *DescribeBackendParams) error {
		assert.Equal(t, "json", params.Format)
		return nil
	})

	SetConfigInitializer(mockCI)
	SetProvisioner(mockProv)

	err := executeDescribe(t.Context(), &describeRequest{Component: "vpc", Stack: "dev", Format: "json"})
	require.NoError(t, err)
}
