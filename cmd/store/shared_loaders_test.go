package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	pstore "github.com/cloudposse/atmos/pkg/store"
)

// writeMinimalStoreProject writes a self-contained Atmos project (config only, no stacks) into a
// temp dir and returns its path. `atmos store` commands never process stacks (InitCliConfig is
// called with processStacks=false), so a bare atmos.yaml with no auth or stores configured is
// enough for loadService/loadServiceForList to resolve fully in-process.
func writeMinimalStoreProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	full := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(full, []byte(`base_path: "."
`), 0o644))
	return dir
}

// writeStoreProjectWithInvalidLogLevel is writeMinimalStoreProject but with an unparsable
// logs.level, which fails InitCliConfig's checkConfig step regardless of processStacks -- unlike
// the missing stacks.base_path/included_paths checks (which only apply when processStacks is
// true), this is the one InitCliConfig failure store commands can actually hit.
func writeStoreProjectWithInvalidLogLevel(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	full := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(full, []byte(`base_path: "."
logs:
  level: not-a-real-level
`), 0o644))
	return dir
}

func TestLoadService_Success(t *testing.T) {
	t.Chdir(writeMinimalStoreProject(t))

	svc, err := loadService(storeScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)
	require.NotNil(t, svc)

	// The project declares no stores, so List reports none -- confirming a real, queryable service
	// was built.
	assert.Empty(t, svc.List())
}

func TestLoadService_InitConfigError(t *testing.T) {
	t.Chdir(writeStoreProjectWithInvalidLogLevel(t))

	_, err := loadService(storeScope{Stack: "dev", Component: "vpc"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)
}

// TestLoadService_AuthError covers buildStoreAuthManager's error branch: an explicit --identity
// with no auth configured in atmos.yaml makes auth.CreateAndAuthenticateManager fail rather than
// silently skip authentication, and loadService propagates that error unwrapped.
func TestLoadService_AuthError(t *testing.T) {
	t.Chdir(writeMinimalStoreProject(t))

	_, err := loadService(storeScope{Stack: "dev", Component: "vpc", Identity: "explicit-identity"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthNotConfigured)
}

func TestLoadServiceForList_Success(t *testing.T) {
	t.Chdir(writeMinimalStoreProject(t))

	// loadServiceForList never authenticates, so it succeeds even with an explicit identity that
	// would fail loadService -- it never reaches buildStoreAuthManager.
	svc, err := loadServiceForList(storeScope{Stack: "dev", Component: "vpc", Identity: "explicit-identity"})
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.Empty(t, svc.List())
}

func TestLoadServiceForList_InitConfigError(t *testing.T) {
	t.Chdir(writeStoreProjectWithInvalidLogLevel(t))

	_, err := loadServiceForList(storeScope{Stack: "dev", Component: "vpc"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)
}

func TestBuildStoreAuthManager_Error(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	_, err := buildStoreAuthManager(atmosConfig, storeScope{Stack: "dev", Identity: "explicit-identity"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthNotConfigured)
}

func TestBuildStoreAuthManager_Success(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// No identity requested and no auth configured -- CreateAndAuthenticateManager returns a nil
	// manager and no error (no authentication).
	authManager, err := buildStoreAuthManager(atmosConfig, storeScope{Stack: "dev"})
	require.NoError(t, err)
	assert.Nil(t, authManager)
}

// TestLoadServiceSeams drives deps.go's loadServiceFn/loadServiceForListFn directly (not through
// installService's override) so their real closure bodies -- the wrap-and-return `if err != nil`
// lines -- execute, not just the underlying loaders.
func TestLoadServiceSeams_Success(t *testing.T) {
	t.Chdir(writeMinimalStoreProject(t))

	svc, err := loadServiceFn(storeScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)
	require.NotNil(t, svc)

	svc2, err := loadServiceForListFn(storeScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)
	require.NotNil(t, svc2)
}

func TestLoadServiceSeams_Error(t *testing.T) {
	t.Chdir(writeStoreProjectWithInvalidLogLevel(t))

	_, err := loadServiceFn(storeScope{Stack: "dev", Component: "vpc"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)

	_, err = loadServiceForListFn(storeScope{Stack: "dev", Component: "vpc"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)
}

// Compile-time guard: the tests above assume *pstore.Service satisfies storeService structurally;
// a signature change to either interface should fail the build here rather than surface as an
// obscure runtime mismatch.
var _ storeService = (*pstore.Service)(nil)
