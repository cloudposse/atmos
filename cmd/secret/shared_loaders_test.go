package secret

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	storepkg "github.com/cloudposse/atmos/pkg/store"
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

func TestInjectSecretStoreAuthResolver_ResolverOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	authManager := authtypes.NewMockAuthManager(ctrl)
	mockStore := storepkg.NewMockIdentityAwareStore(ctrl)

	authManager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{})
	mockStore.EXPECT().
		SetAuthContext(gomock.Not(nil), "").
		Do(func(resolver storepkg.AuthContextResolver, identityName string) {
			assert.NotNil(t, resolver)
			assert.Empty(t, identityName)
		})

	atmosConfig := &schema.AtmosConfiguration{
		Stores: storepkg.StoreRegistry{
			"explicit-identity-store": mockStore,
		},
	}

	injectSecretStoreAuthResolver(atmosConfig, authManager)
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

func TestLoadServiceForList_VerifyFalse_Success(t *testing.T) {
	t.Chdir(writeMinimalAtmosProject(t))

	// verify=false is the credential-free path: it resolves the component with auth disabled and
	// builds the service from declarations alone — no identity authentication, no store resolver.
	svc, err := loadServiceForList(secretScope{Stack: "dev", Component: "vpc"}, false)
	require.NoError(t, err)
	require.NotNil(t, svc)

	// The component declares no secrets, so the service is real but has no declarations.
	assert.Empty(t, svc.Declarations())
	assert.False(t, svc.IsDeclared("ANY_SECRET"))
}

func TestLoadServiceForList_VerifyTrue_Delegates(t *testing.T) {
	t.Chdir(writeMinimalAtmosProject(t))

	// verify=true delegates to loadService, which authenticates; the no-auth/no-secret component
	// still resolves fully in-process, so a real service is returned.
	svc, err := loadServiceForList(secretScope{Stack: "dev", Component: "vpc"}, true)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.Empty(t, svc.Declarations())
}

func TestLoadServiceForList_InitConfigError(t *testing.T) {
	// An empty dir has no Atmos config, so InitCliConfig fails before any component work, on both
	// the verify=false branch and (via loadService) the verify=true branch.
	t.Chdir(t.TempDir())

	_, err := loadServiceForList(secretScope{Stack: "dev", Component: "vpc"}, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)

	_, err = loadServiceForList(secretScope{Stack: "dev", Component: "vpc"}, true)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)
}

func TestLoadServiceForListSeam_Success(t *testing.T) {
	t.Chdir(writeMinimalAtmosProject(t))

	// The deps.go seam wrapper delegates to the real loader; drive it directly (both verify values)
	// so its wrapping logic is covered, not just the underlying function.
	svc, err := loadServiceForListFn(secretScope{Stack: "dev", Component: "vpc"}, false)
	require.NoError(t, err)
	require.NotNil(t, svc)

	svc2, err := loadServiceForListFn(secretScope{Stack: "dev", Component: "vpc"}, true)
	require.NoError(t, err)
	require.NotNil(t, svc2)
}

func TestLoadServiceForListSeam_Error(t *testing.T) {
	t.Chdir(t.TempDir())

	_, err := loadServiceForListFn(secretScope{Stack: "dev", Component: "vpc"}, false)
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
