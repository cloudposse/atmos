package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

// secretsSection is a tiny helper that builds a `secrets:` section with a single SOPS-backed
// declaration carrying the given extra spec fields. It mirrors the resolved component-section
// shape the stack processor feeds to the secrets merge.
func secretsSection(name string, extra map[string]any) map[string]any {
	spec := map[string]any{"sops": "vault"}
	for k, v := range extra {
		spec[k] = v
	}
	return map[string]any{"vars": map[string]any{name: spec}}
}

// secretScope reaches into a merged secrets section and returns the stamped scope for a named var.
func secretScopeOf(t *testing.T, section map[string]any, name string) string {
	t.Helper()
	vars, ok := section["vars"].(map[string]any)
	require.True(t, ok, "secrets.vars must be a map")
	spec, ok := vars[name].(map[string]any)
	require.True(t, ok, "secret %q must be a map", name)
	scope, _ := spec["scope"].(string)
	return scope
}

// TestTagSecretsScopes covers the position→scope stamping helper directly: the global layer is
// stack-scoped, the component-level layers are instance-scoped, and an explicit conflicting scope
// is rejected as invalid component secrets.
func TestTagSecretsScopes(t *testing.T) {
	t.Run("stamps-position-derived-scopes", func(t *testing.T) {
		global := secretsSection("DB", nil)
		base := secretsSection("BASE", nil)
		component := secretsSection("COMP", nil)
		overrides := secretsSection("OVR", nil)

		layers, err := tagSecretsScopes(global, base, component, overrides)
		require.NoError(t, err)
		require.Len(t, layers, 4)

		assert.Equal(t, string(secrets.ScopeStack), secretScopeOf(t, layers[0], "DB"), "global layer is stack-scoped")
		assert.Equal(t, string(secrets.ScopeInstance), secretScopeOf(t, layers[1], "BASE"), "base layer is instance-scoped")
		assert.Equal(t, string(secrets.ScopeInstance), secretScopeOf(t, layers[2], "COMP"), "component layer is instance-scoped")
		assert.Equal(t, string(secrets.ScopeInstance), secretScopeOf(t, layers[3], "OVR"), "overrides layer is instance-scoped")
	})

	t.Run("does-not-mutate-input", func(t *testing.T) {
		global := secretsSection("DB", nil)
		_, err := tagSecretsScopes(global, nil, nil, nil)
		require.NoError(t, err)
		// The shared stack-level section must not gain a stamped scope in place.
		_, present := global["vars"].(map[string]any)["DB"].(map[string]any)["scope"]
		assert.False(t, present, "tagSecretsScopes must not mutate the input section")
	})

	t.Run("explicit-conflicting-scope-rejected", func(t *testing.T) {
		// A component-level declaration (positionally instance-scoped) that pins itself to
		// stack scope is a one-way-rule violation.
		component := secretsSection("DB", map[string]any{"scope": string(secrets.ScopeStack)})
		_, err := tagSecretsScopes(nil, nil, component, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidComponentSecrets)
		assert.ErrorIs(t, err, secrets.ErrScopeConflict)
	})
}

// TestMergeComponentConfigurations_Secrets covers the per-component secrets merge added by the
// secrets-management feature: global→base→component→overrides precedence, position-derived scope
// stamping, "most-specific wins" scope resolution, conflict rejection, section omission, isolation,
// and availability across all component types.
func TestMergeComponentConfigurations_Secrets(t *testing.T) {
	atmosCfg := &schema.AtmosConfiguration{}

	t.Run("no-secrets-anywhere-omits-section", func(t *testing.T) {
		opts := ComponentProcessorOptions{
			ComponentType: cfg.TerraformComponentType,
			Component:     "vpc",
			AtmosConfig:   atmosCfg,
		}
		comp, err := mergeComponentConfigurations(atmosCfg, &opts, minimalComponentResult())
		require.NoError(t, err)
		_, present := comp[cfg.SecretsSectionName]
		assert.False(t, present, "secrets must be absent when no layer declares any")
	})

	t.Run("global-only-is-stack-scoped", func(t *testing.T) {
		opts := ComponentProcessorOptions{
			ComponentType: cfg.TerraformComponentType,
			Component:     "vpc",
			AtmosConfig:   atmosCfg,
			GlobalSecrets: secretsSection("DB", nil),
		}
		comp, err := mergeComponentConfigurations(atmosCfg, &opts, minimalComponentResult())
		require.NoError(t, err)
		section, ok := comp[cfg.SecretsSectionName].(map[string]any)
		require.True(t, ok, "secrets section must be present and a map")
		assert.Equal(t, string(secrets.ScopeStack), secretScopeOf(t, section, "DB"))
	})

	t.Run("base-only-flows-through", func(t *testing.T) {
		opts := ComponentProcessorOptions{
			ComponentType: cfg.TerraformComponentType,
			Component:     "vpc",
			AtmosConfig:   atmosCfg,
		}
		res := minimalComponentResult()
		res.BaseComponentSecrets = secretsSection("INHERITED", nil)
		comp, err := mergeComponentConfigurations(atmosCfg, &opts, res)
		require.NoError(t, err)
		section, ok := comp[cfg.SecretsSectionName].(map[string]any)
		require.True(t, ok, "inherited secrets must flow through")
		assert.Equal(t, string(secrets.ScopeInstance), secretScopeOf(t, section, "INHERITED"))
	})

	t.Run("component-redeclaring-global-pulls-to-instance-scope", func(t *testing.T) {
		// Most-specific wins: a stack-level secret re-declared at the component level becomes
		// instance-scoped.
		opts := ComponentProcessorOptions{
			ComponentType: cfg.TerraformComponentType,
			Component:     "vpc",
			AtmosConfig:   atmosCfg,
			GlobalSecrets: secretsSection("DB", nil),
		}
		res := minimalComponentResult()
		res.ComponentSecrets = secretsSection("DB", nil)
		comp, err := mergeComponentConfigurations(atmosCfg, &opts, res)
		require.NoError(t, err)
		section := comp[cfg.SecretsSectionName].(map[string]any)
		assert.Equal(t, string(secrets.ScopeInstance), secretScopeOf(t, section, "DB"),
			"component re-declaration overrides the inherited stack scope")
	})

	t.Run("overrides-win-over-component-and-base", func(t *testing.T) {
		opts := ComponentProcessorOptions{
			ComponentType: cfg.TerraformComponentType,
			Component:     "vpc",
			AtmosConfig:   atmosCfg,
			GlobalSecrets: secretsSection("DB", map[string]any{"description": "g"}),
		}
		res := minimalComponentResult()
		res.BaseComponentSecrets = secretsSection("DB", map[string]any{"description": "b"})
		res.ComponentSecrets = secretsSection("DB", map[string]any{"description": "c"})
		res.ComponentOverridesSecrets = secretsSection("DB", map[string]any{"description": "o"})
		comp, err := mergeComponentConfigurations(atmosCfg, &opts, res)
		require.NoError(t, err)
		section := comp[cfg.SecretsSectionName].(map[string]any)
		spec := section["vars"].(map[string]any)["DB"].(map[string]any)
		assert.Equal(t, "o", spec["description"], "overrides win on scalar fields")
		assert.Equal(t, string(secrets.ScopeInstance), spec["scope"], "instance-scoped after component re-declaration")
	})

	t.Run("explicit-scope-conflict-is-rejected", func(t *testing.T) {
		opts := ComponentProcessorOptions{
			ComponentType: cfg.TerraformComponentType,
			Component:     "vpc",
			AtmosConfig:   atmosCfg,
		}
		res := minimalComponentResult()
		// Component layer pinning a secret to stack scope conflicts with its instance position.
		res.ComponentSecrets = secretsSection("DB", map[string]any{"scope": string(secrets.ScopeStack)})
		_, err := mergeComponentConfigurations(atmosCfg, &opts, res)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidComponentSecrets)
	})

	t.Run("available-for-all-component-types", func(t *testing.T) {
		for _, ct := range []string{cfg.TerraformComponentType, cfg.HelmfileComponentType, cfg.PackerComponentType} {
			opts := ComponentProcessorOptions{
				ComponentType: ct,
				Component:     "vpc",
				AtmosConfig:   atmosCfg,
				GlobalSecrets: secretsSection("DB", nil),
			}
			comp, err := mergeComponentConfigurations(atmosCfg, &opts, minimalComponentResult())
			require.NoError(t, err, "component type %q", ct)
			_, present := comp[cfg.SecretsSectionName]
			assert.True(t, present, "secrets section must be present for component type %q", ct)
		}
	})

	t.Run("result-mutation-does-not-leak-into-source-maps", func(t *testing.T) {
		opts := ComponentProcessorOptions{
			ComponentType: cfg.TerraformComponentType,
			Component:     "vpc",
			AtmosConfig:   atmosCfg,
		}
		baseSecrets := secretsSection("DB", map[string]any{"description": "base"})
		compSecrets := secretsSection("DB", map[string]any{"description": "component"})
		res := minimalComponentResult()
		res.BaseComponentSecrets = baseSecrets
		res.ComponentSecrets = compSecrets
		comp, err := mergeComponentConfigurations(atmosCfg, &opts, res)
		require.NoError(t, err)

		merged := comp[cfg.SecretsSectionName].(map[string]any)["vars"].(map[string]any)["DB"].(map[string]any)
		merged["description"] = "mutated"

		assert.Equal(t, "base", baseSecrets["vars"].(map[string]any)["DB"].(map[string]any)["description"],
			"base source map must stay intact")
		assert.Equal(t, "component", compSecrets["vars"].(map[string]any)["DB"].(map[string]any)["description"],
			"component source map must stay intact")
	})

	t.Run("source-mutation-does-not-leak-into-merged-result", func(t *testing.T) {
		opts := ComponentProcessorOptions{
			ComponentType: cfg.TerraformComponentType,
			Component:     "vpc",
			AtmosConfig:   atmosCfg,
		}
		baseSecrets := secretsSection("DB", map[string]any{"description": "base"})
		compSecrets := secretsSection("DB", map[string]any{"description": "component"})
		res := minimalComponentResult()
		res.BaseComponentSecrets = baseSecrets
		res.ComponentSecrets = compSecrets
		comp, err := mergeComponentConfigurations(atmosCfg, &opts, res)
		require.NoError(t, err)
		merged := comp[cfg.SecretsSectionName].(map[string]any)["vars"].(map[string]any)["DB"].(map[string]any)
		require.Equal(t, "component", merged["description"], "component wins over base before mutation")

		// Mutate the source maps after merge; the merged result must be unaffected.
		baseSecrets["vars"].(map[string]any)["DB"].(map[string]any)["description"] = "mutated-base"
		compSecrets["vars"].(map[string]any)["DB"].(map[string]any)["description"] = "mutated-component"

		assert.Equal(t, "component", merged["description"],
			"mutating source maps after merge must not affect the merged result")
	})
}
