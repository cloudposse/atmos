package secret

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/secrets"
)

// TestDeclaredNames proves the declared secret names come back sorted.
func TestDeclaredNames(t *testing.T) {
	svc := newFakeSecretService()
	svc.declarations = []secrets.Declaration{{Name: "C_KEY"}, {Name: "A_KEY"}, {Name: "B_KEY"}}

	assert.Equal(t, []string{"A_KEY", "B_KEY", "C_KEY"}, declaredNames(svc))
}

// TestResolveSetName_Inline parses the positional NAME and NAME=VALUE forms.
func TestResolveSetName_Inline(t *testing.T) {
	svc := newFakeSecretService()

	t.Run("name and value", func(t *testing.T) {
		got, err := resolveSetName(svc, []string{"DB=secret"})
		require.NoError(t, err)
		assert.Equal(t, "DB", got.name)
		assert.Equal(t, "secret", got.value)
		assert.True(t, got.hasValue)
	})

	t.Run("name only", func(t *testing.T) {
		got, err := resolveSetName(svc, []string{"DB"})
		require.NoError(t, err)
		assert.Equal(t, "DB", got.name)
		assert.False(t, got.hasValue)
	})

	t.Run("empty name is rejected", func(t *testing.T) {
		_, err := resolveSetName(svc, []string{"=value"})
		require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
	})
}

// TestResolveSetName_PromptNonInteractive proves that with no positional arg and no interactive
// terminal, the prompt path falls back to the standard "NAME required" error.
func TestResolveSetName_PromptNonInteractive(t *testing.T) {
	svc := newFakeSecretService()
	svc.declarations = []secrets.Declaration{{Name: "DB"}}

	_, err := resolveSetName(svc, nil)
	require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

// TestGuardInstanceOverride covers all three branches of the stack-scope override guard.
func TestGuardInstanceOverride(t *testing.T) {
	t.Run("no component passes through", func(t *testing.T) {
		svc := newFakeSecretService()
		require.NoError(t, guardInstanceOverride(svc, secretScope{Stack: "prod"}, "SHARED"))
	})

	t.Run("stack-scoped at a component is rejected", func(t *testing.T) {
		svc := newFakeSecretService()
		svc.scopes = map[string]secrets.Scope{"SHARED": secrets.ScopeStack}
		err := guardInstanceOverride(svc, secretScope{Stack: "prod", Component: "api"}, "SHARED")
		require.ErrorIs(t, err, secrets.ErrSecretNotOverridable)
	})

	t.Run("instance-scoped passes through", func(t *testing.T) {
		svc := newFakeSecretService()
		svc.declared = map[string]bool{"INST": true}
		require.NoError(t, guardInstanceOverride(svc, secretScope{Stack: "prod", Component: "api"}, "INST"))
	})
}
