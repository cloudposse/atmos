package secret

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/stretchr/testify/require"
)

func TestSecretDiscoveryDefersExecutableSelectors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses the touch command")
	}
	for _, native := range []bool{true, false} {
		t.Run(fmt.Sprintf("native=%v", native), func(t *testing.T) {
			dir := writeMinimalAtmosProject(t)
			configPath := filepath.Join(dir, "atmos.yaml")
			config, err := os.ReadFile(configPath)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(configPath, append(config, []byte("templates:\n  settings: {enabled: true}\n")...), 0o644))
			marker := filepath.Join(dir, "selector-executed")
			selector := "!exec touch " + marker
			if !native {
				selector = fmt.Sprintf("%q", selector)
			}
			manifest := "vars: {stage: dev}\ncomponents:\n  terraform:\n    vpc:\n      secrets:\n        vars:\n          DYNAMIC:\n            sops: " + selector + "\n"
			require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "deploy", "dev.yaml"), []byte(manifest), 0o644))
			t.Chdir(dir)
			scope := secretScope{Stack: "dev", Component: "vpc"}
			listed, err := loadServiceForList(scope, false)
			require.NoError(t, err)
			statuses := listed.Status(false)
			require.Len(t, statuses, 1)
			require.NoFileExists(t, marker, "listing must not execute a backend selector")
			require.True(t, statuses[0].Unresolved)
			entries, _, err := enumerateSecretScopes(secretScope{Stack: "dev"})
			require.NoError(t, err)
			require.Len(t, entries, 1)
			require.NoFileExists(t, marker, "enumeration must not execute a backend selector")
			svc, err := loadService(scope)
			require.NoError(t, err)
			require.NoFileExists(t, marker, "loading a service must keep unused declarations lazy")
			_, err = svc.Get("DYNAMIC", secrets.ResolveOptions{})
			require.Error(t, err, "touch returns no configured provider name")
			require.FileExists(t, marker, "an explicit read may evaluate its own selector")
		})
	}
}

func TestSecretDiscoveryDefersExecutableDependencies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses the touch command")
	}
	for _, field := range []string{"template dependency", "provider configuration"} {
		t.Run(field, func(t *testing.T) {
			dir := writeMinimalAtmosProject(t)
			configPath := filepath.Join(dir, "atmos.yaml")
			config, err := os.ReadFile(configPath)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(configPath, append(config, []byte("templates:\n  settings: {enabled: true}\n")...), 0o644))
			marker := filepath.Join(dir, "selector-executed")
			declaration := fmt.Sprintf("      vars:\n        backend: !exec touch %s\n      secrets:\n        vars:\n          DYNAMIC:\n            sops: '{{ .vars.backend }}'\n", marker)
			if field == "provider configuration" {
				declaration = fmt.Sprintf("      secrets:\n        providers:\n          local:\n            kind: sops/age\n            spec:\n              file: !exec touch %s\n        vars:\n          DYNAMIC:\n            sops: local\n", marker)
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
