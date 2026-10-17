package secrets

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
)

const cfnSopsSelector = "!aws.cloudformation.output producer dev SopsProviderName"

// selectorSopsSection builds a component section with one selector-backed SOPS declaration and the
// given SOPS provider definitions.
func selectorSopsSection(providerDefs map[string]any, sopsValue string) map[string]any {
	secretsSection := map[string]any{
		"vars": map[string]any{
			"API_KEY": map[string]any{"sops": sopsValue},
		},
	}
	if providerDefs != nil {
		secretsSection["providers"] = providerDefs
	}
	return map[string]any{"secrets": secretsSection}
}

func ageProvider(spec map[string]any) map[string]any {
	return map[string]any{"kind": "sops/age", "spec": spec}
}

func TestResolveFileDependencies_UnresolvedSelectorIsConservative(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		providers   map[string]any
		wantFiles   []string
		wantFolders []string
	}{
		{
			name: "every known provider contributes its folder",
			providers: map[string]any{
				"a": ageProvider(map[string]any{"path": "secrets/a"}),
				"b": ageProvider(map[string]any{"path": "secrets/b"}),
				// Same folder twice de-duplicates.
				"c": ageProvider(map[string]any{"path": "secrets/b/"}),
			},
			wantFolders: []string{filepath.Join("secrets", "a"), filepath.Join("secrets", "b")},
		},
		{
			name: "file template contributes the folder of its static prefix",
			providers: map[string]any{
				"a": ageProvider(map[string]any{"file": "vault/{{ .atmos_stack }}/{{ .atmos_component }}.enc.yaml"}),
			},
			wantFolders: []string{"vault"},
		},
		{
			name: "literal file is an exact file",
			providers: map[string]any{
				"a": ageProvider(map[string]any{"file": "vault/shared.enc.yaml"}),
			},
			wantFiles: []string{"vault/shared.enc.yaml"},
		},
		{
			name:        "no provider definitions falls back to the default directory",
			wantFolders: []string{"secrets"},
		},
		{
			name: "atmos.yaml providers are included",
			atmosConfig: &schema.AtmosConfiguration{Secrets: schema.SecretsConfig{Providers: map[string]schema.SecretProviderConfig{
				"global": {Kind: "sops/age", Spec: map[string]any{"path": "global-secrets"}},
			}}},
			providers: map[string]any{
				"local": ageProvider(map[string]any{"path": "local-secrets"}),
			},
			wantFolders: []string{"global-secrets", "local-secrets"},
		},
		{
			name: "a provider whose path is a selector widens to the working directory",
			providers: map[string]any{
				"a": ageProvider(map[string]any{"path": cfnSopsSelector}),
			},
			wantFolders: []string{"."},
		},
		{
			name: "a provider whose spec is a selector widens to the working directory",
			providers: map[string]any{
				"a": map[string]any{"kind": "sops/age", "spec": cfnSopsSelector},
			},
			wantFolders: []string{"."},
		},
		{
			name: "a provider that is itself a selector widens to the working directory",
			providers: map[string]any{
				"a": cfnSopsSelector,
			},
			wantFolders: []string{"."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := tt.atmosConfig
			if atmosConfig == nil {
				atmosConfig = &schema.AtmosConfiguration{}
			}
			svc := NewService(atmosConfig, "dev", "consumer", selectorSopsSection(tt.providers, cfnSopsSelector))

			set := svc.ResolveFileDependencies()

			assert.Equal(t, tt.wantFiles, set.Files)
			assert.Equal(t, tt.wantFolders, set.Folders)
			require.Len(t, set.Unresolved, 1, "the unresolved declaration must be reported, never skipped")
			u := set.Unresolved[0]
			assert.Equal(t, "API_KEY", u.Declaration)
			assert.Equal(t, "sops", u.Field)
			assert.Equal(t, cfnSopsSelector, u.Selector)
			assert.ErrorIs(t, u.Err, ErrSelectorUnresolved)
			assert.ErrorIs(t, u.Err, ErrSelectorEvaluatorUnavailable)
			assert.Equal(t, providers.FileLocations{Files: tt.wantFiles, Folders: tt.wantFolders}, u.Locations)
		})
	}
}

// TestResolveFileDependencies_ProviderDefinitionSelector covers a literal `sops:` name whose provider
// definition holds the selector: only that provider widens, and the report names the provider.
func TestResolveFileDependencies_ProviderDefinitionSelector(t *testing.T) {
	section := selectorSopsSection(map[string]any{
		"dev-sops":   ageProvider(map[string]any{"file": cfnSopsSelector}),
		"other-sops": ageProvider(map[string]any{"path": "other"}),
	}, "dev-sops")

	set := NewService(&schema.AtmosConfiguration{}, "dev", "consumer", section).ResolveFileDependencies()

	assert.Equal(t, []string{"."}, set.Folders, "the unrelated provider must not contribute")
	require.Len(t, set.Unresolved, 1)
	assert.Equal(t, "providers.dev-sops", set.Unresolved[0].Field)
	assert.Contains(t, set.Unresolved[0].Selector, cfnSopsSelector)
}

// TestResolveFileDependencies_EvaluatorResolvesExactFile proves the exact file is used (no folders,
// nothing unresolved) when the selector can be resolved, and that a failing evaluator falls back to
// the conservative locations (the recovery must not trigger when it succeeds).
func TestResolveFileDependencies_EvaluatorResolvesExactFile(t *testing.T) {
	section := selectorSopsSection(map[string]any{
		"dev-sops": ageProvider(map[string]any{"path": "vault"}),
	}, cfnSopsSelector)

	t.Run("resolved selector yields the exact file", func(t *testing.T) {
		evaluator := func(path []string, raw any) (any, error) {
			assert.Equal(t, []string{"secrets", "vars", "API_KEY", "sops"}, path)
			assert.Equal(t, cfnSopsSelector, raw)
			return "dev-sops", nil
		}
		set := NewService(&schema.AtmosConfiguration{}, "dev", "consumer", section, WithSelectorEvaluator(evaluator)).ResolveFileDependencies()

		assert.Equal(t, []string{filepath.Join("vault", "dev.consumer.enc.yaml")}, set.Files)
		assert.Empty(t, set.Folders)
		assert.Empty(t, set.Unresolved)
	})

	t.Run("failing evaluator falls back to conservative folders", func(t *testing.T) {
		boom := errors.New("producer not deployed")
		evaluator := func([]string, any) (any, error) { return nil, boom }
		set := NewService(&schema.AtmosConfiguration{}, "dev", "consumer", section, WithSelectorEvaluator(evaluator)).ResolveFileDependencies()

		assert.Empty(t, set.Files)
		assert.Equal(t, []string{"vault"}, set.Folders)
		require.Len(t, set.Unresolved, 1)
		assert.ErrorIs(t, set.Unresolved[0].Err, boom)
	})
}

// TestResolveFileDependencies_NotAffectedByStoreSelectors proves a selector on a store-backed
// declaration (no file) and an unrelated resolution failure add no conservative locations.
func TestResolveFileDependencies_NotAffectedByStoreSelectors(t *testing.T) {
	section := map[string]any{
		"secrets": map[string]any{
			"vars": map[string]any{
				"STORE_KEY":   map[string]any{"store": cfnSopsSelector},
				"MISSING_KEY": map[string]any{"sops": "does-not-exist"},
			},
		},
	}

	set := NewService(&schema.AtmosConfiguration{}, "dev", "consumer", section).ResolveFileDependencies()

	assert.Empty(t, set.Files)
	assert.Empty(t, set.Folders)
	assert.Empty(t, set.Unresolved)
}

// TestFileDependencies_LiteralSopsUnchanged proves exact-file behavior for literal SOPS declarations
// is unchanged: only the exact file, no folders, nothing unresolved.
func TestFileDependencies_LiteralSopsUnchanged(t *testing.T) {
	cfg, section, file := newSopsServiceConfig(t, true)

	set := NewService(cfg, "dev", "api", section).ResolveFileDependencies()

	assert.Equal(t, []string{file}, set.Files)
	assert.Empty(t, set.Folders)
	assert.Empty(t, set.Unresolved)
}
