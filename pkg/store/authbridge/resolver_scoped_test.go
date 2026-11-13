package authbridge

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestScopedResolverKeepsIdentityOwnership(t *testing.T) {
	for _, tt := range []struct {
		name                               string
		auth                               *schema.AuthContext
		operationError, cleanupError, want error
	}{
		{name: "success", auth: &schema.AuthContext{Environment: map[string]string{"VAULT_TOKEN": "synthetic-reader"}}},
		{name: "missing context", want: store.ErrAuthContextNotAvailable},
		{name: "empty environment", auth: &schema.AuthContext{}, want: store.ErrAuthContextNotAvailable},
		{name: "failed operation", auth: &schema.AuthContext{Environment: map[string]string{"VAULT_TOKEN": "synthetic-reader"}}, operationError: assert.AnError, want: assert.AnError},
		{name: "failed cleanup", auth: &schema.AuthContext{Environment: map[string]string{"VAULT_TOKEN": "synthetic-reader"}}, cleanupError: assert.AnError, want: assert.AnError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			scoped := types.NewMockScopedAuthManager(ctrl)
			manager := struct {
				types.AuthManager
				types.ScopedAuthManager
			}{types.NewMockAuthManager(ctrl), scoped}
			events := []string{}
			scoped.EXPECT().WithIsolatedIdentity(gomock.Any(), "reader", gomock.Any()).DoAndReturn(
				func(ctx context.Context, _ string, operation func(context.Context, *schema.AuthContext) error) error {
					events = append(events, "acquired")
					err := operation(ctx, tt.auth)
					events = append(events, "released")
					return errors.Join(err, tt.cleanupError)
				})
			resolver := NewResolver(manager, &schema.ConfigAndStacksInfo{})
			err := resolver.WithEnvironmentAuthContext(t.Context(), "reader", func(ctx context.Context, auth *store.EnvironmentAuthConfig) error {
				events = append(events, "operation")
				require.Same(t, t.Context(), ctx)
				require.Equal(t, map[string]string{"VAULT_TOKEN": "synthetic-reader"}, auth.Environment)
				auth.Environment["VAULT_TOKEN"] = "changed"
				require.Equal(t, "synthetic-reader", tt.auth.Environment["VAULT_TOKEN"], "operation mutations must not reach the identity context")
				tt.auth.Environment["ISSUER_ONLY"] = "unshared"
				require.NotContains(t, auth.Environment, "ISSUER_ONLY", "identity mutations must not reach the operation context")
				return tt.operationError
			})
			if tt.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.want)
			}
			expected := []string{"acquired", "released"}
			if tt.auth != nil && len(tt.auth.Environment) != 0 {
				expected = []string{"acquired", "operation", "released"}
			}
			require.Equal(t, expected, events, "the caller must regain control only after owned cleanup")
		})
	}
}

func TestScopedResolverRejectsUnsupportedManagerAndNilOperation(t *testing.T) {
	ctrl := gomock.NewController(t)
	base := types.NewMockAuthManager(ctrl)
	operation := func(context.Context, *store.EnvironmentAuthConfig) error {
		t.Fatal("operation must not run")
		return nil
	}
	require.ErrorIs(t, NewResolver(base, nil).WithEnvironmentAuthContext(t.Context(), "reader", operation), store.ErrAuthContextNotAvailable)
	scoped := types.NewMockScopedAuthManager(ctrl)
	manager := struct {
		types.AuthManager
		types.ScopedAuthManager
	}{base, scoped}
	require.ErrorIs(t, NewResolver(manager, nil).WithEnvironmentAuthContext(t.Context(), "reader", nil), store.ErrAuthContextNotAvailable)
}
