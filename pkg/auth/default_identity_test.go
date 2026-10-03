package auth

import (
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// forceNonInteractive makes identity selection behave as it does without a TTY.
func forceNonInteractive(t *testing.T) {
	t.Helper()
	previous := viper.GetBool("interactive")
	t.Cleanup(func() { viper.Set("interactive", previous) })
	viper.Set("interactive", false)
}

// TestGetDefaultIdentityNonInteractiveErrors lists conflicting defaults and tells the user how to resolve them.
func TestGetDefaultIdentityNonInteractiveErrors(t *testing.T) {
	forceNonInteractive(t)
	m := &manager{config: &schema.AuthConfig{Identities: map[string]schema.Identity{
		"zeta": {Default: true}, "alpha": {Default: true}, "other": {},
	}}}

	t.Run("multiple defaults", func(t *testing.T) {
		_, err := m.GetDefaultIdentity(false)
		require.ErrorIs(t, err, errUtils.ErrMultipleDefaultIdentities)
		assert.Contains(t, strings.Join(cockroachErrors.GetAllDetails(err), " "), "alpha, zeta")
		hints := strings.Join(cockroachErrors.GetAllHints(err), " ")
		assert.Contains(t, hints, "--identity=<name>")
		assert.Contains(t, hints, "remove `default: true`")
	})

	t.Run("bare identity flag without a terminal", func(t *testing.T) {
		_, err := m.GetDefaultIdentity(true)
		require.ErrorIs(t, err, errUtils.ErrIdentitySelectionRequiresTTY)
		assert.ErrorIs(t, err, errUtils.ErrTTYRequired)
		assert.Contains(t, strings.Join(cockroachErrors.GetAllHints(err), " "), "--identity=<name>")
		available, _ := errUtils.GetContext(err, "available_identities")
		assert.Equal(t, "alpha,other,zeta", available)
	})
}

// TestAutoDetectDefaultIdentityOutcomes keeps "no defaults" non-fatal while conflicting defaults are errors.
func TestAutoDetectDefaultIdentityOutcomes(t *testing.T) {
	forceNonInteractive(t)
	for _, tc := range []struct {
		name       string
		identities map[string]schema.Identity
		want       string
		wantErr    error
	}{
		{"one default", map[string]schema.Identity{"a": {Kind: "aws/user", Default: true}, "b": {Kind: "aws/user"}}, "a", nil},
		{"no default stays silent", map[string]schema.Identity{"a": {Kind: "aws/user"}, "b": {Kind: "aws/user"}}, "", nil},
		{"conflicting defaults", map[string]schema.Identity{"a": {Kind: "aws/user", Default: true}, "b": {Kind: "aws/user", Default: true}}, "", errUtils.ErrMultipleDefaultIdentities},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := autoDetectDefaultIdentity(&schema.AuthConfig{Identities: tc.identities}, "")
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

// layerWithDefaults builds a raw auth layer marking the named identities default.
func layerWithDefaults(names ...string) map[string]any {
	identities := make(map[string]any, len(names))
	for _, name := range names {
		identities[name] = map[string]any{"default": true, "kind": "aws/user"}
	}
	return map[string]any{"identities": identities}
}

// defaultsOf returns the identities a layer marks `default: true`.
func defaultsOf(layer map[string]any) []string {
	return defaultIdentityNames(layer["identities"].(map[string]any))
}

// TestClearSupersededAuthDefaults applies "more specific wins" across layers without mutating them.
func TestClearSupersededAuthDefaults(t *testing.T) {
	stack, component := layerWithDefaults("dev"), layerWithDefaults("sandbox")
	noDefault := map[string]any{"identities": map[string]any{"other": map[string]any{"kind": "aws/user"}}}

	for _, tc := range []struct {
		name   string
		layers []map[string]any
		want   [][]string
	}{
		{"component supersedes stack", []map[string]any{stack, component}, [][]string{nil, {"sandbox"}}},
		{"stack default survives alone", []map[string]any{stack, noDefault}, [][]string{{"dev"}, nil}},
		{"most specific of three", []map[string]any{stack, component, layerWithDefaults("prod")}, [][]string{nil, nil, {"prod"}}},
		{"empty and nil layers", []map[string]any{nil, {}, component}, [][]string{nil, nil, {"sandbox"}}},
		{"same identity redeclared", []map[string]any{stack, layerWithDefaults("dev")}, [][]string{nil, {"dev"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := ClearSupersededAuthDefaults(tc.layers)
			require.Len(t, result, len(tc.want))
			for i, want := range tc.want {
				if result[i] == nil || result[i]["identities"] == nil {
					assert.Empty(t, want)
					continue
				}
				assert.Equal(t, want, defaultsOf(result[i]), "layer %d", i)
			}
		})
	}

	t.Run("inputs are never mutated", func(t *testing.T) {
		layers := []map[string]any{layerWithDefaults("dev"), layerWithDefaults("sandbox")}
		result := ClearSupersededAuthDefaults(layers)
		assert.Equal(t, []string{"dev"}, defaultsOf(layers[0]), "source isolation")
		// Mutating the result must not reach the inputs either.
		result[0]["identities"].(map[string]any)["dev"].(map[string]any)["kind"] = "changed"
		assert.Equal(t, "aws/user", layers[0]["identities"].(map[string]any)["dev"].(map[string]any)["kind"])
		// And changing an input afterwards must not reach the result.
		layers[0]["identities"].(map[string]any)["dev"].(map[string]any)["default"] = true
		assert.Empty(t, defaultsOf(result[0]))
	})
}
