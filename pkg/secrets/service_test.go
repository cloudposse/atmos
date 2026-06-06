package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
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
