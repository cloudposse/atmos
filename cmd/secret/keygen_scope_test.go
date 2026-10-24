package secret

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	_ "github.com/cloudposse/atmos/pkg/component/mock"
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
	_, err := keygenProviderDefinitions(keygenSections(stacks, ""), secretScope{Stack: "dev"}, []string{"local"})
	require.ErrorIs(t, err, ErrAmbiguousVault)
	// A different component type does not contribute an ambiguous override.
	got, err := keygenProviderDefinitions(keygenSections(stacks, "helm"), secretScope{Stack: "dev", ComponentType: "helm"}, nil)
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

func TestRunSecretKeygen_ProviderOnlyStackAndRegisteredType(t *testing.T) {
	for _, kind := range []string{"stack", "imported", "explicit", "pattern", "filename", "mock", "missing-stack", "no-provider", "missing-component", "missing-type"} {
		t.Run(kind, func(t *testing.T) {
			setupIO(t)
			dir := writeMinimalAtmosProject(t)
			configPath := filepath.Join(dir, "atmos.yaml")
			configData, err := os.ReadFile(configPath)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(configPath, append(configData, []byte("templates:\n  settings:\n    enabled: true\n")...), 0o644))
			keyPath := filepath.Join(dir, "keys.txt")
			t.Setenv("SOPS_AGE_KEY_FILE", keyPath)
			provider := "providers:\n  local:\n    kind: sops/age\n    spec:\n      file: secrets/dev.enc.yaml\n      age_key_file: '{{ .vars.key_path }}'\n"
			vars := fmt.Sprintf("vars: {stage: dev, key_path: %q}\n", keyPath)
			manifest := vars + "secrets:\n" + indentKeygenFixture(provider, "  ")
			args := []string{"keygen", "local", "-s", "dev"}
			switch kind {
			case "missing-component":
				manifest += "components:\n  terraform:\n    existing: {}\n"
				args = append(args, "--component", "missing")
			case "missing-type":
				manifest += "components:\n  terraform:\n    existing: {}\n"
				args = append(args, "--type", "missing")
			case "missing-stack":
				args[len(args)-1] = "missing"
			case "no-provider":
				manifest = vars
			case "imported":
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks", "catalog"), 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "catalog", "provider.yaml"), []byte(manifest), 0o644))
				manifest = "import: [catalog/provider]\n"
			case "explicit":
				manifest = "name: selected\n" + manifest
				args[len(args)-1] = "selected"
			case "pattern", "filename":
				configPath := filepath.Join(dir, "atmos.yaml")
				config, err := os.ReadFile(configPath)
				require.NoError(t, err)
				naming := "  name_pattern: '{stage}'"
				if kind == "filename" {
					naming = ""
					args[len(args)-1] = "deploy/dev"
				}
				config = []byte(strings.ReplaceAll(string(config), "  name_template: \"{{.vars.stage}}\"", naming))
				require.NoError(t, os.WriteFile(configPath, config, 0o644))
			}
			if kind == "mock" {
				manifest = vars + "components:\n  mock:\n    vpc:\n      secrets:\n" + indentKeygenFixture(provider, "        ")
				args = append(args, "--type", "mock")
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "deploy", "dev.yaml"), []byte(manifest), 0o644))
			t.Chdir(dir)
			if kind == "missing-stack" || kind == "no-provider" || kind == "missing-component" || kind == "missing-type" {
				require.ErrorIs(t, runSecretSubcommand(t, args...), ErrNoVault)
				require.NoFileExists(t, keyPath)
				return
			}
			require.NoError(t, runSecretSubcommand(t, args...))
			key, err := os.ReadFile(keyPath)
			require.NoError(t, err)
			require.Contains(t, string(key), "AGE-SECRET-KEY")
			recipients, err := os.ReadFile(filepath.Join(dir, ".sops.yaml"))
			require.NoError(t, err)
			require.NotContains(t, string(recipients), "{{")
		})
	}
}

func indentKeygenFixture(value, prefix string) string {
	return prefix + strings.ReplaceAll(strings.TrimSuffix(value, "\n"), "\n", "\n"+prefix) + "\n"
}
