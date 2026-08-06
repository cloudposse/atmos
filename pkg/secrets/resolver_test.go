package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
	"github.com/cloudposse/atmos/pkg/store"
)

// newSecretTestConfig builds an AtmosConfiguration with a single `secret: true` store named
// "app-secrets" backed by the provided mock store, plus a component section declaring DATADOG.
func newSecretTestConfig(s store.Store) (*schema.AtmosConfiguration, map[string]any) {
	cfg := &schema.AtmosConfiguration{
		StoresConfig: store.StoresConfig{
			"app-secrets": store.StoreConfig{Type: "aws-ssm-parameter-store", Secret: true},
		},
		Stores: store.StoreRegistry{"app-secrets": s},
	}
	componentSection := map[string]any{
		"secrets": map[string]any{
			"vars": map[string]any{
				"DATADOG_API_KEY": map[string]any{
					"store":    "app-secrets",
					"required": true,
				},
			},
		},
	}
	return cfg, componentSection
}

// TestResolve_MaskOnly_SkipsRetrieval proves that on an inspection command with masking
// enabled, the resolver returns the mask replacement WITHOUT contacting the backend.
func TestResolve_MaskOnly_SkipsRetrieval(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	// Critical assertion: Get must NEVER be called in mask-only mode.
	mockStore.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	cfg, componentSection := newSecretTestConfig(mockStore)
	require.NoError(t, iolib.Initialize())

	info := &schema.ConfigAndStacksInfo{
		Stack:            "prod",
		Component:        "api",
		ComponentSection: componentSection,
		SecretsMaskOnly:  true,
	}

	got, err := Resolve(cfg, "!secret DATADOG_API_KEY", "prod", info)
	require.NoError(t, err)
	assert.Equal(t, iolib.GetContext().Masker().Replacement(), got)
}

// TestResolve_MaskOnly_RejectsUndeclared proves that masked inspection validates the local
// declaration registry without contacting the backend.
func TestResolve_MaskOnly_RejectsUndeclared(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	cfg, componentSection := newSecretTestConfig(mockStore)
	require.NoError(t, iolib.Initialize())

	info := &schema.ConfigAndStacksInfo{
		Stack:            "prod",
		Component:        "api",
		ComponentSection: componentSection,
		SecretsMaskOnly:  true,
	}

	_, err := Resolve(cfg, "!secret UNDECLARED_KEY", "prod", info)
	require.ErrorIs(t, err, ErrSecretNotDeclared)
}

// TestResolve_RealValue retrieves the real value when masking does not skip retrieval.
func TestResolve_RealValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().
		Get("prod", "api", "DATADOG_API_KEY").
		Return("dd-secret-value", nil).
		Times(1)

	cfg, componentSection := newSecretTestConfig(mockStore)
	require.NoError(t, iolib.Initialize())

	info := &schema.ConfigAndStacksInfo{
		Stack:            "prod",
		Component:        "api",
		ComponentSection: componentSection,
		SecretsMaskOnly:  false,
	}

	got, err := Resolve(cfg, "!secret DATADOG_API_KEY", "prod", info)
	require.NoError(t, err)
	assert.Equal(t, "dd-secret-value", got)
}

// TestResolve_Undeclared errors when the secret is not declared in the component section.
func TestResolve_Undeclared(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	cfg, componentSection := newSecretTestConfig(mockStore)

	info := &schema.ConfigAndStacksInfo{
		Stack:            "prod",
		Component:        "api",
		ComponentSection: componentSection,
	}

	_, err := Resolve(cfg, "!secret UNDECLARED_KEY", "prod", info)
	require.ErrorIs(t, err, ErrSecretNotDeclared)
}

// TestResolve_DefaultOnMissing falls back to the default modifier when retrieval fails.
func TestResolve_DefaultOnMissing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().
		Get("prod", "api", "DATADOG_API_KEY").
		Return(nil, assertErr{}).
		Times(1)

	cfg, componentSection := newSecretTestConfig(mockStore)

	info := &schema.ConfigAndStacksInfo{
		Stack:            "prod",
		Component:        "api",
		ComponentSection: componentSection,
	}

	got, err := Resolve(cfg, `!secret DATADOG_API_KEY | default "dev-key"`, "prod", info)
	require.NoError(t, err)
	assert.Equal(t, "dev-key", got)
}

func TestResolve_RawDefaultDoesNotHideUnsupportedCapability(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().
		Get("prod", "api", "DATADOG_API_KEY").
		Return(map[string]any{"enabled": true}, nil).
		Times(1)

	cfg, componentSection := newSecretTestConfig(mockStore)
	info := &schema.ConfigAndStacksInfo{
		Stack:            "prod",
		Component:        "api",
		ComponentSection: componentSection,
	}

	_, err := Resolve(cfg, `!secret DATADOG_API_KEY | raw | default "fallback"`, "prod", info)
	require.ErrorIs(t, err, providers.ErrRawNotSupported)
}

func TestResolve_RawDefaultUsesFallbackOnMissing(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().
		Get("prod", "api", "DATADOG_API_KEY").
		Return(nil, store.ErrResourceNotFound).
		Times(1)

	cfg, componentSection := newSecretTestConfig(mockStore)
	info := &schema.ConfigAndStacksInfo{
		Stack:            "prod",
		Component:        "api",
		ComponentSection: componentSection,
	}

	got, err := Resolve(cfg, `!secret DATADOG_API_KEY | raw | default "fallback"`, "prod", info)
	require.NoError(t, err)
	assert.Equal(t, "fallback", got)
}

// assertErr is a trivial error used to simulate a backend miss.
type assertErr struct{}

func (assertErr) Error() string { return "not found" }

// TestParseSecretArgs covers name + modifier parsing.
func TestParseSecretArgs(t *testing.T) {
	defaultValue := "localhost"
	tests := []struct {
		name            string
		input           string
		expectedName    string
		expectedRaw     bool
		expectedDefault *string
		expectedErr     error
	}{
		{
			name:            "raw with default",
			input:           `!secret DB_CONFIG | raw | default "localhost"`,
			expectedName:    "DB_CONFIG",
			expectedRaw:     true,
			expectedDefault: &defaultValue,
		},
		{name: "compact raw", input: `!secret DB_CONFIG |raw`, expectedName: "DB_CONFIG", expectedRaw: true},
		{name: "raw path conflict", input: `!secret DB_CONFIG | raw | path ".host"`, expectedErr: ErrInvalidSecretArgs},
		{name: "empty name", input: "!secret ", expectedErr: ErrEmptyName},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, opts, err := parseSecretArgs(tt.input)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedName, name)
			assert.Equal(t, tt.expectedRaw, opts.Raw)
			assert.Equal(t, tt.expectedDefault, opts.Default)
		})
	}
}
