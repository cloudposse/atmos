package secret

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/secrets"
)

func TestRunSecretSet_StackScopeWithoutComponent(t *testing.T) {
	svc := newFakeSecretService()
	svc.scopes = map[string]secrets.Scope{"TOKEN": secrets.ScopeStack}
	installService(t, svc, nil)
	overrideEnumerateScopes(t, []scopeEntry{{Stack: "dev", Component: "vpc", ComponentType: "terraform", Section: secretDeclarationSection("TOKEN", map[string]any{"sops": "local", "scope": "stack"})}}, nil)
	require.NoError(t, runSecretSubcommand(t, "set", "TOKEN=value", "-s", "dev"))
	require.Len(t, svc.setCalls, 1)
	require.Equal(t, "TOKEN", svc.setCalls[0].name)
	require.Equal(t, "value", svc.setCalls[0].value)
}

func TestRunSecretValidate_StackScopeWithoutComponent(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)
	overrideEnumerateScopes(t, []scopeEntry{{Stack: "dev", Component: "vpc", ComponentType: "terraform", Section: secretDeclarationSection("TOKEN", map[string]any{"store": "local", "scope": "stack"})}}, nil)
	require.NoError(t, runSecretSubcommand(t, "validate", "-s", "dev"))
}

// Exercise real scope stamping, SOPS encryption and validation without a component flag.
func TestSecretStackScope_SopsRoundTrip(t *testing.T) {
	setupIO(t)
	dir := writeMinimalAtmosProject(t)
	identity, err := age.GenerateX25519Identity()
	require.NoError(t, err)
	t.Setenv("SOPS_AGE_KEY", identity.String())
	manifest := fmt.Sprintf(`vars: {stage: dev}
secrets:
  providers:
    local:
      kind: sops/age
      spec:
        age_recipients: %s
  vars:
    TOKEN: {sops: local, required: true}
components:
  terraform:
    vpc:
      secrets:
        vars:
          PRIVATE: {sops: local, required: true}
`, identity.Recipient().String())
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "deploy", "dev.yaml"), []byte(manifest), 0o644))
	t.Chdir(dir)
	require.ErrorIs(t, runSecretSubcommand(t, "validate", "-s", "dev"), errUtils.ErrValidationFailed)
	require.NoError(t, runSecretSubcommand(t, "set", "TOKEN=value", "-s", "dev"))
	require.NoError(t, runSecretSubcommand(t, "validate", "-s", "dev"))
	// Shared validation must not silently count the uninitialized instance secret as validated.
	require.ErrorIs(t, runSecretSubcommand(t, "validate", "-s", "dev", "-c", "vpc"), errUtils.ErrValidationFailed)
	require.ErrorIs(t, runSecretSubcommand(t, "set", "PRIVATE=value", "-s", "dev"), errUtils.ErrRequiredFlagNotProvided)
	svc, err := loadService(secretScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)
	value, err := svc.Get("TOKEN", secrets.ResolveOptions{})
	require.NoError(t, err)
	require.Equal(t, "value", value)
}

func TestFindSharedSetContext_DifferentProvidersRequireComponent(t *testing.T) {
	entry := func(component, file string) scopeEntry {
		section := secretDeclarationSection("TOKEN", map[string]any{"sops": "local", "scope": "stack"})
		section["secrets"].(map[string]any)["providers"] = map[string]any{"local": map[string]any{"kind": "sops/age", "spec": map[string]any{"file": file}}}
		return scopeEntry{Stack: "dev", Component: component, ComponentType: "terraform", Section: section}
	}
	overrideEnumerateScopes(t, []scopeEntry{entry("a", "a.enc.yaml"), entry("b", "b.enc.yaml")}, nil)
	_, _, err := findSharedSetContext(secretScope{Stack: "dev"}, "TOKEN")
	require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}
