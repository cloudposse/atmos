package secret

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Scoped providers must be discoverable before any secret has been declared.
func TestRunSecretKeygen_ScopedProviders(t *testing.T) {
	for _, component := range []bool{false, true} {
		t.Run(fmt.Sprintf("component=%t", component), func(t *testing.T) {
			setupIO(t)
			dir := writeMinimalAtmosProject(t)
			keyPath := filepath.Join(dir, "keys.txt")
			t.Setenv("SOPS_AGE_KEY_FILE", keyPath)
			provider := fmt.Sprintf("providers:\n      local:\n        kind: sops/age\n        spec:\n          file: secrets/dev.enc.yaml\n          age_key_file: %q\n", keyPath)
			manifest := "vars: {stage: dev}\nsecrets:\n  " + provider + "components:\n  terraform:\n    vpc: {}\n"
			args := []string{"keygen", "local", "-s", "dev"}
			if component {
				manifest = fmt.Sprintf("vars: {stage: dev}\ncomponents:\n  terraform:\n    vpc:\n      secrets:\n        providers:\n          local:\n            kind: sops/age\n            spec:\n              file: secrets/dev.enc.yaml\n              age_key_file: %q\n", keyPath)
				args = append(args, "-c", "vpc", "--type", "terraform")
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "deploy", "dev.yaml"), []byte(manifest), 0o644))
			t.Chdir(dir)
			require.NoError(t, runSecretSubcommand(t, args...))
			key, err := os.ReadFile(keyPath)
			require.NoError(t, err)
			require.Contains(t, string(key), "AGE-SECRET-KEY")
			recipients, err := os.ReadFile(filepath.Join(dir, ".sops.yaml"))
			require.NoError(t, err)
			require.Contains(t, string(recipients), "age1")
		})
	}
}

func TestKeygenProviderDefinitions_AmbiguousOverride(t *testing.T) {
	section := func(file string) map[string]any {
		return map[string]any{"secrets": map[string]any{"providers": map[string]any{"local": map[string]any{"kind": "sops/age", "spec": map[string]any{"file": file}}}}}
	}
	stacks := map[string]any{"dev": map[string]any{"components": map[string]any{"terraform": map[string]any{"a": section("a.enc.yaml"), "b": section("b.enc.yaml")}}}}
	_, err := keygenProviderDefinitions(stacks, secretScope{Stack: "dev"}, []string{"local"})
	require.ErrorIs(t, err, ErrAmbiguousVault)
	// A different component type does not contribute an ambiguous override.
	got, err := keygenProviderDefinitions(stacks, secretScope{Stack: "dev", ComponentType: "helm"}, nil)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestRunSecretKeygen_RejectsMissingScope(t *testing.T) {
	for _, args := range [][]string{
		{"keygen", "-c", "vpc"},
		{"keygen", "-s", "dev", "-c", "missing"},
	} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			setupIO(t)
			t.Chdir(writeMinimalAtmosProject(t))
			require.Error(t, runSecretSubcommand(t, args...))
		})
	}
}

func TestMergeKeygenProviders_PreservesGlobalConfig(t *testing.T) {
	global := map[string]schema.SecretProviderConfig{"local": {Kind: "original"}, "other": {Kind: "unchanged"}}
	config := schema.AtmosConfiguration{Secrets: schema.SecretsConfig{Providers: global}}
	mergeKeygenProviders(&config, map[string]schema.SecretProviderConfig{"local": {Kind: "scoped"}})
	require.Equal(t, "scoped", config.Secrets.Providers["local"].Kind)
	require.Equal(t, "unchanged", config.Secrets.Providers["other"].Kind)
	require.Equal(t, "original", global["local"].Kind)
}
