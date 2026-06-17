package secret

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

// writeMinimalAtmosProject writes a self-contained Atmos project (config + one stack + one
// terraform component) into a temp dir and returns its path. The component declares no auth and no
// secrets, so loadService resolves it fully in-process — InitCliConfig, a no-op auth manager, and
// component description — without any cloud credentials.
func writeMinimalAtmosProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	write := func(rel, content string) {
		full := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}

	write("atmos.yaml", `base_path: "."
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_template: "{{.vars.stage}}"
`)
	write("stacks/deploy/dev.yaml", `vars:
  stage: dev
components:
  terraform:
    vpc:
      vars:
        name: myvpc
`)
	write("components/terraform/vpc/main.tf", "# vpc component.\n")
	return dir
}

func TestLoadService_Success(t *testing.T) {
	t.Chdir(writeMinimalAtmosProject(t))

	svc, err := loadService(secretScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)
	require.NotNil(t, svc)

	// The component declares no secrets, so the service has no declarations and reports unknown
	// names as not declared — confirming a real, queryable service was built.
	assert.Empty(t, svc.Declarations())
	assert.False(t, svc.IsDeclared("ANY_SECRET"))
}

func TestLoadServiceAndConfig_Success(t *testing.T) {
	t.Chdir(writeMinimalAtmosProject(t))

	svc, atmosConfig, err := loadServiceAndConfig(secretScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)
	require.NotNil(t, svc)
	require.NotNil(t, atmosConfig)
	assert.NotEmpty(t, atmosConfig.BasePath, "the resolved config must carry the project base path")
}

func TestLoadService_InitConfigError(t *testing.T) {
	// An empty dir has no Atmos config, so InitCliConfig fails before any component work.
	t.Chdir(t.TempDir())

	_, err := loadService(secretScope{Stack: "dev", Component: "vpc"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)
}

func TestSecretStoreDefaultIdentity(t *testing.T) {
	t.Run("explicit identity wins", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		authManager := authtypes.NewMockAuthManager(ctrl)

		got := secretStoreDefaultIdentity(secretScope{Identity: "cli-admin"}, authManager)
		assert.Equal(t, "cli-admin", got)
	})

	t.Run("manager default fills empty identity", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		authManager := authtypes.NewMockAuthManager(ctrl)
		authManager.EXPECT().GetDefaultIdentity(false).Return("default-admin", nil)

		got := secretStoreDefaultIdentity(secretScope{}, authManager)
		assert.Equal(t, "default-admin", got)
	})

	t.Run("select sentinel falls back to auth chain", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		authManager := authtypes.NewMockAuthManager(ctrl)
		authManager.EXPECT().GetDefaultIdentity(false).Return("", errors.New("no default"))
		authManager.EXPECT().GetChain().Return([]string{"provider", "chain-admin"})

		got := secretStoreDefaultIdentity(secretScope{Identity: cfg.IdentityFlagSelectValue}, authManager)
		assert.Equal(t, "chain-admin", got)
	})

	t.Run("disabled sentinel falls back to auth chain", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		authManager := authtypes.NewMockAuthManager(ctrl)
		authManager.EXPECT().GetDefaultIdentity(false).Return("", errors.New("no default"))
		authManager.EXPECT().GetChain().Return([]string{"provider", "chain-admin"})

		got := secretStoreDefaultIdentity(secretScope{Identity: cfg.IdentityFlagDisabledValue}, authManager)
		assert.Equal(t, "chain-admin", got)
	})
}

func TestLoadServiceAndConfig_ComponentNotFound(t *testing.T) {
	t.Chdir(writeMinimalAtmosProject(t))

	// The stack exists but the component does not, so buildAuthManager's component description
	// fails — exercising the auth-load error path.
	_, _, err := loadServiceAndConfig(secretScope{Stack: "dev", Component: "missing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load component config for auth")
}

func TestLoadServiceSeams_Success(t *testing.T) {
	t.Chdir(writeMinimalAtmosProject(t))

	// The deps.go seam wrappers delegate to the real loaders; drive them directly so their wrapping
	// logic is covered, not just the underlying functions.
	svc, err := loadServiceFn(secretScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)
	require.NotNil(t, svc)

	svc2, atmosConfig, err := loadServiceAndConfigFn(secretScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)
	require.NotNil(t, svc2)
	require.NotNil(t, atmosConfig)
}

func TestLoadServiceSeams_Error(t *testing.T) {
	t.Chdir(t.TempDir())

	_, err := loadServiceFn(secretScope{Stack: "dev", Component: "vpc"})
	require.Error(t, err)

	_, _, err = loadServiceAndConfigFn(secretScope{Stack: "dev", Component: "vpc"})
	require.Error(t, err)
}

func TestBuildAuthManager_ComponentNotFound(t *testing.T) {
	t.Chdir(writeMinimalAtmosProject(t))

	// Resolve a real config, then drive buildAuthManager directly with a missing component to cover
	// its error branch independently of the loaders.
	_, atmosConfig, err := loadServiceAndConfig(secretScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)

	_, err = buildAuthManager(atmosConfig, secretScope{Stack: "dev", Component: "missing"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load component config for auth")
}
