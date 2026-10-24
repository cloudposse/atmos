package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
	"github.com/cloudposse/atmos/pkg/store"
)

// serviceTestConfig builds a config + section with one store-backed declaration.
func serviceTestConfig(s store.Store) (*schema.AtmosConfiguration, map[string]any) {
	cfg := &schema.AtmosConfiguration{
		StoresConfig: store.StoresConfig{
			"app-secrets": store.StoreConfig{Type: "aws-ssm-parameter-store", Secret: true},
		},
		Stores: store.StoreRegistry{"app-secrets": s},
	}
	section := map[string]any{
		"secrets": map[string]any{
			"vars": map[string]any{
				"API_KEY": map[string]any{"store": "app-secrets", "required": true},
			},
		},
	}
	return cfg, section
}

// TestService_Status_RemoteGatedByVerify proves that a remote (non-local) store-backed secret is
// reported as Unknown without verification — its backend is never contacted — and is checked only
// when verify=true. This is what makes `atmos secret list` credential-free by default.
func TestService_Status_RemoteGatedByVerify(t *testing.T) {
	t.Run("verify_false_reports_unknown_without_contacting_backend", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// No EXPECT on Has: gomock fails the test if the backend is contacted.
		mockStatus := store.NewMockStatusStore(ctrl)
		cfg, section := serviceTestConfig(mockStatus)
		svc := NewService(cfg, "prod", "api", section)

		statuses := svc.Status(false)
		require.Len(t, statuses, 1)
		assert.True(t, statuses[0].Unknown, "remote store status must be unknown without --verify")
		assert.False(t, statuses[0].Initialized)
		require.NoError(t, statuses[0].Err)
	})

	t.Run("verify_true_contacts_backend", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockStatus := store.NewMockStatusStore(ctrl)
		mockStatus.EXPECT().Has("prod", "api", "API_KEY").Return(true, nil)
		cfg, section := serviceTestConfig(mockStatus)
		svc := NewService(cfg, "prod", "api", section)

		statuses := svc.Status(true)
		require.Len(t, statuses, 1)
		assert.False(t, statuses[0].Unknown)
		assert.True(t, statuses[0].Initialized)
		require.NoError(t, statuses[0].Err)
	})
}

func TestService_SetGetDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().Set("prod", "api", "API_KEY", "v1").Return(nil)
	mockStore.EXPECT().Get("prod", "api", "API_KEY").Return("v1", nil)

	cfg, section := serviceTestConfig(mockStore)
	svc := NewService(cfg, "prod", "api", section)

	require.NoError(t, svc.Set("API_KEY", "v1"))

	got, err := svc.Get("API_KEY", ResolveOptions{})
	require.NoError(t, err)
	assert.Equal(t, "v1", got)
}

func TestService_StoreReferenceOverridesSecretName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().
		Set("prod", "api", "op://{{ .atmos_stack }}/{{ .atmos_component }}/password", "v1").
		Return(nil)
	mockStore.EXPECT().
		Get("prod", "api", "op://{{ .atmos_stack }}/{{ .atmos_component }}/password").
		Return("v1", nil)

	cfg := &schema.AtmosConfiguration{
		StoresConfig: store.StoresConfig{
			"op": store.StoreConfig{Type: "onepassword", Secret: true},
		},
		Stores: store.StoreRegistry{"op": mockStore},
	}
	section := map[string]any{
		"secrets": map[string]any{
			"vars": map[string]any{
				"DB_PASSWORD": map[string]any{
					"store":     "op",
					"reference": "op://{{ .atmos_stack }}/{{ .atmos_component }}/password",
				},
			},
		},
	}
	svc := NewService(cfg, "prod", "api", section)

	require.NoError(t, svc.Set("DB_PASSWORD", "v1"))

	got, err := svc.Get("DB_PASSWORD", ResolveOptions{})
	require.NoError(t, err)
	assert.Equal(t, "v1", got)
}

func TestService_Set_Undeclared(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	cfg, section := serviceTestConfig(mockStore)
	svc := NewService(cfg, "prod", "api", section)

	err := svc.Set("NOT_DECLARED", "x")
	require.ErrorIs(t, err, ErrSecretNotDeclared)
}

func TestService_Delete_Unsupported(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Plain MockStore does not implement DeletableStore.
	mockStore := store.NewMockStore(ctrl)
	cfg, section := serviceTestConfig(mockStore)
	svc := NewService(cfg, "prod", "api", section)

	err := svc.Delete("API_KEY")
	require.ErrorIs(t, err, ErrDeleteNotSupported)
}

func TestService_Validate_MissingRequired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	// Status falls back to Get for a plain store; simulate a miss.
	mockStore.EXPECT().Get("prod", "api", "API_KEY").Return(nil, assertErr{})

	cfg, section := serviceTestConfig(mockStore)
	svc := NewService(cfg, "prod", "api", section)

	result := svc.Validate()
	assert.False(t, result.Valid())
	require.Len(t, result.MissingRequired, 1)
	assert.Equal(t, "API_KEY", result.MissingRequired[0].Declaration.Name)
}

// globalServiceTestConfig builds a config + section with one global-scoped store-backed
// declaration (as a shared catalog fragment would declare it).
func globalServiceTestConfig(s store.Store) (*schema.AtmosConfiguration, map[string]any) {
	cfg := &schema.AtmosConfiguration{
		StoresConfig: store.StoresConfig{
			"app-secrets": store.StoreConfig{Type: "aws-ssm-parameter-store", Secret: true},
		},
		Stores: store.StoreRegistry{"app-secrets": s},
	}
	section := map[string]any{
		"secrets": map[string]any{
			"vars": map[string]any{
				"SHARED_CLIENT_SECRET": map[string]any{"store": "app-secrets", "scope": "global"},
			},
		},
	}
	return cfg, section
}

// TestService_GlobalScope_Converges proves a global secret resolves to the same backend
// coordinate (empty stack and component) from every (stack, component) scope: a value written
// via one scope is read back via a completely different one.
func TestService_GlobalScope_Converges(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	// Both services must hit the identical coordinate: ("", "", KEY).
	mockStore.EXPECT().Set("", "", "SHARED_CLIENT_SECRET", "v1").Return(nil)
	mockStore.EXPECT().Get("", "", "SHARED_CLIENT_SECRET").Return("v1", nil)

	cfg, section := globalServiceTestConfig(mockStore)
	writer := NewService(cfg, "prod", "api", section)
	reader := NewService(cfg, "dev", "web", section)

	require.NoError(t, writer.Set("SHARED_CLIENT_SECRET", "v1"))

	got, err := reader.Get("SHARED_CLIENT_SECRET", ResolveOptions{})
	require.NoError(t, err)
	assert.Equal(t, "v1", got)
}

// TestService_GlobalScope_StatusCoordinate proves Status reports the global coordinate (no
// stack/component segments) so list/validate display the real storage location.
func TestService_GlobalScope_StatusCoordinate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().Get("", "", "SHARED_CLIENT_SECRET").Return("v1", nil)

	cfg, section := globalServiceTestConfig(mockStore)
	svc := NewService(cfg, "prod", "api", section)

	// verify=true: contact the backend so a remote store reports a real initialized/missing
	// status (credential-free listing would otherwise mark it Unknown).
	statuses := svc.Status(true)
	require.Len(t, statuses, 1)
	st := statuses[0]
	require.NoError(t, st.Err)
	assert.True(t, st.Initialized)
	assert.Empty(t, st.Coordinate.Stack)
	assert.Empty(t, st.Coordinate.Component)
	assert.Equal(t, "SHARED_CLIENT_SECRET", st.Coordinate.Key)
	assert.Equal(t, ScopeGlobal, st.Coordinate.Scope)
}

// TestService_GlobalScope_SopsUnsupported proves a SOPS-backed global declaration is rejected
// with ErrScopeUnsupported before any read or write (SOPS file placement is scope-keyed and has
// no global derivation rule yet).
func TestService_GlobalScope_SopsUnsupported(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		Secrets: schema.SecretsConfig{
			Providers: map[string]schema.SecretProviderConfig{
				"default": {Kind: "sops/age", Spec: map[string]any{}},
			},
		},
	}
	section := map[string]any{
		"secrets": map[string]any{
			"vars": map[string]any{
				"SHARED": map[string]any{"sops": "default", "scope": "global"},
			},
		},
	}
	svc := NewService(cfg, "prod", "api", section)

	err := svc.Set("SHARED", "v")
	require.ErrorIs(t, err, providers.ErrScopeUnsupported)
}

func TestProviderFor_StoreNotSecret(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfg := &schema.AtmosConfiguration{
		StoresConfig: store.StoresConfig{
			"plain": store.StoreConfig{Type: "aws-ssm-parameter-store", Secret: false},
		},
		Stores: store.StoreRegistry{"plain": store.NewMockStore(ctrl)},
	}
	decl := Declaration{Name: "API_KEY", BackendType: BackendStore, BackendName: "plain"}
	_, err := providerFor(cfg, &decl, nil)
	require.ErrorIs(t, err, ErrStoreNotSecret)
}
