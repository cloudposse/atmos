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

func TestCreateCmd_Structure(t *testing.T) {
	testCommandStructure(t, &commandTestParams{
		cmd:              createCmd,
		parser:           createParser,
		expectedUse:      "create [component]",
		expectedShort:    "Provision the template-packaging backend bucket",
		requiredFlags:    []string{"target"},
		hasPositionalArg: true,
	})
}

func TestCreateCmd_Init(t *testing.T) {
	assert.NotNil(t, createParser, "createParser should be initialized")
	assert.NotNil(t, createCmd, "createCmd should be initialized")
	assert.False(t, createCmd.DisableFlagParsing, "DisableFlagParsing should be false")

	stackFlag := createCmd.Flags().Lookup("stack")
	assert.NotNil(t, stackFlag, "stack flag should be registered")

	identityFlag := createCmd.Flags().Lookup("identity")
	assert.NotNil(t, identityFlag, "identity flag should be registered")

	targetFlag := createCmd.Flags().Lookup("target")
	assert.NotNil(t, targetFlag, "target flag should be registered")
}

func TestExecuteCreateOrUpdate_RequiresStack(t *testing.T) {
	err := executeCreateOrUpdate(t.Context(), createOrUpdateArgs{Component: "vpc", Stack: "", Identity: "", Target: "", AutoApprove: true})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

func TestExecuteCreateOrUpdate_ConfigInitError(t *testing.T) {
	t.Cleanup(ResetDependencies)

	ctrl := gomock.NewController(t)
	mockCI := NewMockConfigInitializer(ctrl)
	mockCI.EXPECT().InitConfigAndAuth("vpc", "dev", "").Return(nil, nil, errors.New("boom"))
	SetConfigInitializer(mockCI)

	err := executeCreateOrUpdate(t.Context(), createOrUpdateArgs{Component: "vpc", Stack: "dev", Identity: "", Target: "", AutoApprove: true})
	require.Error(t, err)
}

func TestExecuteCreateOrUpdate_DescribeComponentError(t *testing.T) {
	t.Cleanup(ResetDependencies)

	ctrl := gomock.NewController(t)
	mockCI := NewMockConfigInitializer(ctrl)
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{}
	mockCI.EXPECT().InitConfigAndAuth("vpc", "dev", "").Return(atmosConfig, info, nil)
	mockCI.EXPECT().DescribeComponent(atmosConfig, info, "vpc", "dev").Return(nil, errors.New("boom"))
	SetConfigInitializer(mockCI)

	err := executeCreateOrUpdate(t.Context(), createOrUpdateArgs{Component: "vpc", Stack: "dev", Identity: "", Target: "", AutoApprove: true})
	require.Error(t, err)
}

func TestExecuteCreateOrUpdate_Success(t *testing.T) {
	t.Cleanup(ResetDependencies)

	ctrl := gomock.NewController(t)
	mockCI := NewMockConfigInitializer(ctrl)
	mockProv := NewMockProvisioner(ctrl)

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{}
	componentConfig := singleS3TargetComponentConfig()

	mockCI.EXPECT().InitConfigAndAuth("vpc", "dev", "my-identity").Return(atmosConfig, info, nil)
	mockCI.EXPECT().DescribeComponent(atmosConfig, info, "vpc", "dev").Return(componentConfig, nil)
	mockProv.EXPECT().CreateBackend(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, params *CreateBackendParams) error {
		assert.Equal(t, "vpc", params.Component)
		assert.Equal(t, "dev", params.Stack)
		assert.Equal(t, componentConfig, params.ComponentConfig)
		assert.Equal(t, "artifacts", params.Target)
		return nil
	})

	SetConfigInitializer(mockCI)
	SetProvisioner(mockProv)

	err := executeCreateOrUpdate(t.Context(), createOrUpdateArgs{Component: "vpc", Stack: "dev", Identity: "my-identity", Target: "artifacts", AutoApprove: true})
	require.NoError(t, err)
}

// confirmExistingBackendOverwrite must skip the existence check entirely when
// autoApprove is set — BackendExists is not expected/mocked, so an
// unexpected call would fail the test via gomock.
func TestConfirmExistingBackendOverwrite_AutoApproveSkipsCheck(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockProv := NewMockProvisioner(ctrl)
	t.Cleanup(ResetDependencies)
	SetProvisioner(mockProv)

	err := confirmExistingBackendOverwrite(t.Context(), &CreateBackendParams{Component: "vpc"}, true)
	require.NoError(t, err)
}

// confirmExistingBackendOverwrite must not prompt when the bucket doesn't
// exist yet — nothing to confirm on a fresh create.
func TestConfirmExistingBackendOverwrite_DoesNotExist_NoPrompt(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockProv := NewMockProvisioner(ctrl)
	mockProv.EXPECT().BackendExists(gomock.Any(), gomock.Any()).Return(false, nil)
	t.Cleanup(ResetDependencies)
	SetProvisioner(mockProv)

	err := confirmExistingBackendOverwrite(t.Context(), &CreateBackendParams{Component: "vpc"}, false)
	require.NoError(t, err)
}

// confirmExistingBackendOverwrite must propagate a BackendExists failure.
func TestConfirmExistingBackendOverwrite_BackendExistsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockProv := NewMockProvisioner(ctrl)
	mockProv.EXPECT().BackendExists(gomock.Any(), gomock.Any()).Return(false, errors.New("access denied"))
	t.Cleanup(ResetDependencies)
	SetProvisioner(mockProv)

	err := confirmExistingBackendOverwrite(t.Context(), &CreateBackendParams{Component: "vpc"}, false)
	require.Error(t, err)
}

// confirmExistingBackendOverwrite must actually reach the confirmation
// prompt when the bucket already exists and autoApprove is unset — proven by
// the non-interactive test environment surfacing ErrInteractiveNotAvailable
// (the real prompt path), rather than the check being silently skipped.
func TestConfirmExistingBackendOverwrite_Exists_ReachesPrompt(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockProv := NewMockProvisioner(ctrl)
	mockProv.EXPECT().BackendExists(gomock.Any(), gomock.Any()).Return(true, nil)
	t.Cleanup(ResetDependencies)
	SetProvisioner(mockProv)

	err := confirmExistingBackendOverwrite(t.Context(), &CreateBackendParams{Component: "vpc"}, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInteractiveNotAvailable)
}
