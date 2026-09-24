package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Repeated references must reuse fully evaluated values without rereading secrets.
// The real describe pipeline also proves static targets never authenticate.
func TestDeferredReferenceValueCache(t *testing.T) {
	loaders := nativeSecretReferenceLoaders()
	for _, name := range []string{"terraform.state", "atmos.Component"} {
		t.Run(name, func(t *testing.T) {
			ac, mockStore := setupNativeSecretReference(t)
			deferred.ConfigureAuth(ac, "")
			calls := 0
			mockStore.EXPECT().Get("dev", "producer", "CREDENTIAL").DoAndReturn(func(_, _, _ string) (any, error) {
				calls++
				return nativeReferenceSecret, nil
			}).MinTimes(1)
			value, err := loaders[name](ac)
			require.NoError(t, err)
			require.Equal(t, nativeReferenceSecret, value)
			before := calls
			value, err = loaders[name](ac)
			require.NoError(t, err)
			require.Equal(t, nativeReferenceSecret, value)
			require.Equal(t, before, calls, "cached target must not be evaluated again")
			deferred.ConfigureAuth(ac, "")
			value, err = loaders[name](ac)
			require.NoError(t, err)
			require.Equal(t, nativeReferenceSecret, value)
			require.Greater(t, calls, before, "a new invocation must read fresh values")
		})
	}
}

func TestDeferredStaticStateCachePreservesMissingOutputErrors(t *testing.T) {
	ac, mockStore := setupNativeSecretReference(t)
	deferred.ConfigureAuth(ac, "")
	mockStore.EXPECT().Get("dev", "producer", "CREDENTIAL").Return(nativeReferenceSecret, nil).MinTimes(1)
	for range 2 {
		_, err := GetTerraformState(ac, "!terraform.state", "dev", "producer", "missing", false, nil, nil)
		require.ErrorIs(t, err, errUtils.ErrReadTerraformState)
	}
	value, err := GetTerraformState(ac, "!terraform.state", "dev", "producer", "credential", false, nil, nil)
	require.NoError(t, err)
	require.Equal(t, nativeReferenceSecret, value)
}

func TestDeferredTargetDoesNotReuseCallerAuthContext(t *testing.T) {
	ac := &schema.AtmosConfiguration{}
	prior := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "previous-account"}}
	require.Same(t, prior, resolvedTargetAuthContext(ac, nil, prior, false), "eager execution retains its fallback")
	deferred.ConfigureAuth(ac, "")
	require.Nil(t, resolvedTargetAuthContext(ac, nil, prior, false))
	manager := &authContextWrapper{stackInfo: &schema.ConfigAndStacksInfo{}}
	require.Nil(t, resolvedTargetAuthContext(ac, manager, prior, false))
	resolved := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: "resolved-account"}}
	manager.stackInfo.AuthContext = resolved
	require.Same(t, resolved, resolvedTargetAuthContext(ac, manager, prior, false))
	require.Nil(t, resolvedTargetAuthContext(ac, manager, prior, true), "disabled auth must discard all Atmos contexts")
}

func setupDeferredCacheTarget(t *testing.T) (*schema.AtmosConfiguration, *deferred.MockAuthFactory, string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{
		"atmos.yaml": `base_path: .
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths: ["**/*"]
`,
		"components/terraform/target/main.tf": "",
		"stacks/dev.yaml": `components:
  terraform:
    target:
      backend_type: local
      backend:
        local:
          path: cached.tfstate
      vars: {}
`,
	} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	t.Setenv("ATMOS_BASE_PATH", dir)
	ClearFindStacksMapCache()
	ClearResolutionContext()
	t.Cleanup(ClearFindStacksMapCache)
	t.Cleanup(ClearResolutionContext)
	ac, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	factory := deferred.NewMockAuthFactory(gomock.NewController(t))
	ac.DeferredAuth = deferred.NewAuthResolver(deferred.AuthOptions{Factory: factory})
	return &ac, factory, filepath.Join(dir, "components", "terraform", "target", "cached.tfstate")
}

func deferredCacheParent(profile string) *schema.ConfigAndStacksInfo {
	return &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"auth": map[string]any{
		"identities": map[string]any{"local": map[string]any{
			"kind": "aws/user", "default": true, "credentials": map[string]any{"profile": profile},
		}},
	}}}
}

func TestDeferredComponentCachesPerEffectiveIdentity(t *testing.T) {
	ac, factory, _ := setupDeferredCacheTarget(t)
	ctrl := gomock.NewController(t)
	executor := NewMockComponentFuncOutputsExecutor(ctrl)
	old := componentFuncOutputsExecutor
	componentFuncOutputsExecutor = executor
	t.Cleanup(func() { componentFuncOutputsExecutor = old })
	for _, profile := range []string{"first", "second"} {
		context := &schema.AuthContext{AWS: &schema.AWSAuthContext{Profile: profile}}
		manager := types.NewMockAuthManager(ctrl)
		manager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{AuthContext: context}).AnyTimes()
		factory.EXPECT().Create(ac, gomock.Any(), "dev").Return(manager, nil).Times(1)
		executor.EXPECT().ExecuteWithSections(ac, "target", "dev", gomock.Any(), context).Return(map[string]any{"id": profile}, nil).Times(1)
		for range 2 {
			value, err := componentFunc(ac, deferredCacheParent(profile), "target", "dev")
			require.NoError(t, err)
			require.Equal(t, profile, value.(map[string]any)["outputs"].(map[string]any)["id"])
		}
	}
}

func TestDeferredComponentDoesNotCacheFailuresOrMaskedValues(t *testing.T) {
	ac, factory, _ := setupDeferredCacheTarget(t)
	factory.EXPECT().Create(ac, gomock.Any(), "dev").Return(nil, nil).Times(1)
	executor := NewMockComponentFuncOutputsExecutor(gomock.NewController(t))
	old := componentFuncOutputsExecutor
	componentFuncOutputsExecutor = executor
	t.Cleanup(func() { componentFuncOutputsExecutor = old })
	gomock.InOrder(
		executor.EXPECT().ExecuteWithSections(ac, "target", "dev", gomock.Any(), nil).Return(nil, errUtils.ErrAuthenticationUnavailable).Times(2),
		executor.EXPECT().ExecuteWithSections(ac, "target", "dev", gomock.Any(), nil).Return(map[string]any{"id": "masked"}, nil),
		executor.EXPECT().ExecuteWithSections(ac, "target", "dev", gomock.Any(), nil).Return(map[string]any{"id": "resolved"}, nil),
		executor.EXPECT().ExecuteWithSections(ac, "target", "dev", gomock.Any(), nil).Return(map[string]any{"id": "masked again"}, nil),
	)
	for range 2 {
		_, err := componentFunc(ac, nil, "target", "dev")
		require.ErrorIs(t, err, errUtils.ErrAuthenticationUnavailable)
	}
	for _, expected := range []string{"masked", "resolved", "resolved", "masked again"} {
		info := &schema.ConfigAndStacksInfo{SecretsMaskOnly: expected != "resolved"}
		value, err := componentFunc(ac, info, "target", "dev")
		require.NoError(t, err)
		require.Equal(t, expected, value.(map[string]any)["outputs"].(map[string]any)["id"])
	}
}

func TestDeferredStateCacheSkipsFailuresAndSeparatesIdentities(t *testing.T) {
	ac, factory, path := setupDeferredCacheTarget(t)
	factory.EXPECT().Create(ac, gomock.Any(), "dev").Return(nil, nil).Times(2)
	load := func(profile string, skip bool) (any, error) {
		var parent auth.AuthManager = &authContextWrapper{stackInfo: deferredCacheParent(profile)}
		return GetTerraformState(ac, "!terraform.state", "dev", "target", "id", skip, nil, parent)
	}
	_, err := load("first", false)
	require.ErrorIs(t, err, errUtils.ErrTerraformStateNotProvisioned)
	require.NoError(t, os.WriteFile(path, []byte(`{"version":4,"outputs":{"id":{"value":"first","type":"string"}}}`), 0o600))
	value, err := load("first", false)
	require.NoError(t, err)
	require.Equal(t, "first", value, "unavailable state must not be cached")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":4,"outputs":{"id":{"value":"second","type":"string"}}}`), 0o600))
	value, err = load("first", false)
	require.NoError(t, err)
	require.Equal(t, "first", value, "repeated references must not reread the backend")
	value, err = load("second", false)
	require.NoError(t, err)
	require.Equal(t, "second", value, "same identity name with changed credentials must read independently")
	value, err = load("first", true)
	require.NoError(t, err)
	require.Equal(t, "second", value, "skipCache must force a fresh backend read")
}
