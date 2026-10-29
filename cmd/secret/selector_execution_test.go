package secret

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/stretchr/testify/require"
)

func TestSecretDiscoveryDefersExecutableSelectors(t *testing.T) {
	tests := []struct {
		name        string
		edition     string
		native      bool
		wantExecute bool
	}{
		{name: "default/native", native: true, wantExecute: true},
		{name: "default/quoted"},
		{name: "current/native", edition: "2026-10-20", native: true, wantExecute: true},
		{name: "current/quoted", edition: "2026-10-20"},
		{name: "legacy/native", edition: "2026-10-19", native: true, wantExecute: true},
		{name: "legacy/quoted", edition: "2026-10-19", wantExecute: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_EDITION", tt.edition)
			dir := writeMinimalAtmosProject(t)
			configPath := filepath.Join(dir, "atmos.yaml")
			config, err := os.ReadFile(configPath)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(configPath, append(config, []byte("templates:\n  settings: {enabled: true}\n")...), 0o644))
			marker := filepath.Join(dir, "selector marker's execution")
			unusedMarker := filepath.Join(dir, "unused selector marker's execution")
			command := selectorHelperCommand(t, marker)
			literalSelector := "!exec " + command
			selector := fmt.Sprintf("!exec %q", command)
			if !tt.native {
				selector = fmt.Sprintf("%q", literalSelector)
			}
			manifest := "vars: {stage: dev}\ncomponents:\n  terraform:\n    vpc:\n      secrets:\n        vars:\n          DYNAMIC:\n            sops: " + selector + "\n"
			manifest += fmt.Sprintf("          UNUSED:\n            sops: !exec %q\n", selectorHelperCommand(t, unusedMarker))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "deploy", "dev.yaml"), []byte(manifest), 0o644))
			t.Chdir(dir)
			scope := secretScope{Stack: "dev", Component: "vpc"}
			listed, err := loadServiceForList(scope, false)
			require.NoError(t, err)
			statuses := listed.Status(false)
			require.Len(t, statuses, 2)
			require.NoFileExists(t, marker, "listing must not execute a backend selector")
			require.NoFileExists(t, unusedMarker)
			for _, status := range statuses {
				require.True(t, status.Unresolved)
			}
			entries, _, err := enumerateSecretScopes(secretScope{Stack: "dev"})
			require.NoError(t, err)
			require.Len(t, entries, 1)
			require.NoFileExists(t, marker, "enumeration must not execute a backend selector")
			require.NoFileExists(t, unusedMarker)
			svc, err := loadService(scope)
			require.NoError(t, err)
			require.NoFileExists(t, marker, "loading a service must keep unused declarations lazy")
			require.NoFileExists(t, unusedMarker)
			_, err = svc.Get("DYNAMIC", secrets.ResolveOptions{})
			require.ErrorIs(t, err, secrets.ErrSelectorResult, "neither empty helper output nor a literal selector names a provider")
			if tt.wantExecute {
				require.FileExists(t, marker, "an explicit read evaluates its own executable selector")
			} else {
				require.NoFileExists(t, marker, "current editions never execute quoted function-like strings")
				require.ErrorContains(t, err, "(got "+literalSelector+")", "the quoted value must remain literal")
			}
			require.NoFileExists(t, unusedMarker, "explicit access must not evaluate a sibling declaration")
		})
	}
}

func TestSecretDiscoveryDefersExecutableDependencies(t *testing.T) {
	for _, field := range []string{"template dependency", "provider configuration"} {
		t.Run(field, func(t *testing.T) {
			dir := writeMinimalAtmosProject(t)
			configPath := filepath.Join(dir, "atmos.yaml")
			config, err := os.ReadFile(configPath)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(configPath, append(config, []byte("templates:\n  settings: {enabled: true}\n")...), 0o644))
			marker := filepath.Join(dir, "selector marker's execution")
			command := selectorHelperCommand(t, marker)
			declaration := fmt.Sprintf("      vars:\n        backend: !exec %q\n      secrets:\n        vars:\n          DYNAMIC:\n            sops: '{{ .vars.backend }}'\n", command)
			if field == "provider configuration" {
				declaration = fmt.Sprintf("      secrets:\n        providers:\n          local:\n            kind: sops/age\n            spec:\n              file: !exec %q\n        vars:\n          DYNAMIC:\n            sops: local\n", command)
			}
			manifest := "vars: {stage: dev}\ncomponents:\n  terraform:\n    vpc:\n" + declaration
			require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "deploy", "dev.yaml"), []byte(manifest), 0o644))
			t.Chdir(dir)
			scope := secretScope{Stack: "dev", Component: "vpc"}
			listed, err := loadServiceForList(scope, false)
			require.NoError(t, err)
			statuses := listed.Status(false)
			require.Len(t, statuses, 1)
			require.NoFileExists(t, marker)
			if field == "template dependency" {
				require.True(t, statuses[0].Unresolved)
			}
			_, _, err = enumerateSecretScopes(secretScope{Stack: "dev"})
			require.NoError(t, err)
			require.NoFileExists(t, marker)
			svc, err := loadService(scope)
			require.NoError(t, err)
			require.NoFileExists(t, marker)
			_, err = svc.Get("DYNAMIC", secrets.ResolveOptions{})
			require.Error(t, err)
			require.FileExists(t, marker)
		})
	}
}

func TestSecretDiscoveryDefersStarlarkSelector(t *testing.T) {
	dir := writeMinimalAtmosProject(t)
	manifest := `vars: {stage: dev}
components:
  terraform:
    vpc:
      secrets:
        vars:
          DYNAMIC:
            sops: !starlark 'fail("selector-executed")'
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "deploy", "dev.yaml"), []byte(manifest), 0o644))
	t.Chdir(dir)
	scope := secretScope{Stack: "dev", Component: "vpc"}
	listed, err := loadServiceForList(scope, false)
	require.NoError(t, err, "discovery must not execute the Starlark fail call")
	statuses := listed.Status(false)
	require.Len(t, statuses, 1)
	require.True(t, statuses[0].Unresolved)
	_, _, err = enumerateSecretScopes(secretScope{Stack: "dev"})
	require.NoError(t, err)
	svc, err := loadService(scope)
	require.NoError(t, err)
	_, err = svc.Get("DYNAMIC", secrets.ResolveOptions{})
	require.ErrorContains(t, err, "selector-executed", "explicit access evaluates the selected declaration")
}
