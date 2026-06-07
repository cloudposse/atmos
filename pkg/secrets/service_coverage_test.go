package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// bareSection builds a component section declaring the given store-backed secret names (no provider
// resolution needed — Declarations/ScopeOf read the raw map).
func bareSection(names ...string) map[string]any {
	vars := map[string]any{}
	for _, n := range names {
		vars[n] = map[string]any{"store": "app-secrets"}
	}
	return map[string]any{"secrets": map[string]any{"vars": vars}}
}

// TestService_Declarations_Sorted proves Declarations returns declarations ordered by name.
func TestService_Declarations_Sorted(t *testing.T) {
	svc := NewService(&schema.AtmosConfiguration{}, "prod", "api", bareSection("B_KEY", "A_KEY", "C_KEY"))

	decls := svc.Declarations()
	require.Len(t, decls, 3)
	assert.Equal(t, "A_KEY", decls[0].Name)
	assert.Equal(t, "B_KEY", decls[1].Name)
	assert.Equal(t, "C_KEY", decls[2].Name)
}

// TestService_Get_DefaultFallback proves a missing backend value falls back to opts.Default
// instead of returning an error.
func TestService_Get_DefaultFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().Get("prod", "api", "API_KEY").Return(nil, assertErr{})

	cfg, section := serviceTestConfig(mockStore)
	svc := NewService(cfg, "prod", "api", section)

	def := "fallback-value"
	got, err := svc.Get("API_KEY", ResolveOptions{Default: &def})
	require.NoError(t, err)
	assert.Equal(t, "fallback-value", got)
}

// TestService_Get_PathExpression proves a YQ path modifier is applied to a structured value.
func TestService_Get_PathExpression(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().Get("prod", "api", "API_KEY").Return(map[string]any{"user": "admin"}, nil)

	cfg, section := serviceTestConfig(mockStore)
	svc := NewService(cfg, "prod", "api", section)

	got, err := svc.Get("API_KEY", ResolveOptions{Path: ".user"})
	require.NoError(t, err)
	assert.Equal(t, "admin", got)
}

// TestService_Status_ProviderError proves a declaration whose backend cannot be resolved is
// reported with a per-secret error rather than failing the whole status sweep.
func TestService_Status_ProviderError(t *testing.T) {
	// No stores configured, so resolving the "missing" store fails.
	cfg := &schema.AtmosConfiguration{}
	section := map[string]any{
		"secrets": map[string]any{
			"vars": map[string]any{
				"API_KEY": map[string]any{"store": "missing"},
			},
		},
	}
	svc := NewService(cfg, "prod", "api", section)

	statuses := svc.Status()
	require.Len(t, statuses, 1)
	require.Error(t, statuses[0].Err)
	assert.False(t, statuses[0].Initialized)
}

// TestService_FileDependencies_DedupesByFile proves the distinct backing files are returned, with
// the two SOPS declarations sharing one file collapsing to a single dependency.
func TestService_FileDependencies_DedupesByFile(t *testing.T) {
	cfg, section, file := newSopsServiceConfig(t, true)
	svc := NewService(cfg, "dev", "api", section)

	deps := svc.FileDependencies()
	require.Len(t, deps, 1, "two declarations sharing one file de-duplicate")
	assert.Equal(t, file, deps[0])
}

// TestService_ScopeOf proves the declared/undeclared branches and the instance-scope default.
func TestService_ScopeOf(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfg, section := serviceTestConfig(store.NewMockStore(ctrl))
	svc := NewService(cfg, "prod", "api", section)

	scope, ok := svc.ScopeOf("API_KEY")
	assert.True(t, ok)
	assert.Equal(t, ScopeInstance, scope, "no explicit scope defaults to instance")

	_, ok = svc.ScopeOf("NOT_DECLARED")
	assert.False(t, ok)
}
