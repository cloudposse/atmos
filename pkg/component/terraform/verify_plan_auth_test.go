package terraform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveVerificationContext_AuthenticatesBeforeProcessingStacks(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte("base_path: .\nstacks:\n  base_path: stacks\n  included_paths: [\"**/*\"]\n  name_pattern: \"{stage}\"\ncomponents:\n  terraform:\n    base_path: components/terraform\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "prod.yaml"), []byte("vars:\n  stage: prod\n"), 0o600))
	t.Chdir(dir)

	ctrl := gomock.NewController(t)
	host := NewMockOperationServices(ctrl)
	manager := types.NewMockAuthManager(ctrl)
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "example", Stack: "prod", Identity: "ci"}

	gomock.InOrder(
		host.EXPECT().SetupTerraformAuth(gomock.Any(), gomock.Any()).Return(manager, nil),
		host.EXPECT().ProcessStacks(gomock.Any(), gomock.Any(), true, true, true, gomock.Any(), auth.AuthManager(manager)).
			Return(schema.ConfigAndStacksInfo{FinalComponent: "example"}, nil),
	)

	vc, err := NewOperations(host).resolveVerificationContext(info)
	require.NoError(t, err)
	assert.Equal(t, "example", vc.processedInfo.FinalComponent)
}

func TestResolveVerificationContext_AuthenticationError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte("base_path: .\nstacks:\n  base_path: stacks\n  included_paths: [\"**/*\"]\n  name_pattern: \"{stage}\"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "prod.yaml"), []byte("vars:\n  stage: prod\n"), 0o600))
	t.Chdir(dir)

	host := NewMockOperationServices(gomock.NewController(t))
	authErr := errors.New("login failed")
	host.EXPECT().SetupTerraformAuth(gomock.Any(), gomock.Any()).Return(nil, authErr)

	_, err := NewOperations(host).resolveVerificationContext(&schema.ConfigAndStacksInfo{ComponentFromArg: "example", Stack: "prod"})
	require.ErrorIs(t, err, authErr)
}
