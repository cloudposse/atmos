package secret

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

// Compile-time guard: these tests depend on the shared-scope fields of secretScope.
var _ = secretScope{SharedOnly: true, Component: "vpc"}

// sharedEntry is an enumerated component whose section declares the given secrets.
func sharedEntry(component string, specs map[string]map[string]any) scopeEntry {
	vars := map[string]any{}
	for name, spec := range specs {
		vars[name] = spec
	}
	return scopeEntry{
		Stack:         "dev",
		Component:     component,
		ComponentType: "terraform",
		Section:       map[string]any{"secrets": map[string]any{"vars": vars}},
	}
}

func stackSpec() map[string]any    { return map[string]any{"sops": "local", "scope": "stack"} }
func globalSpec() map[string]any   { return map[string]any{"sops": "local", "scope": "global"} }
func instanceSpec() map[string]any { return map[string]any{"sops": "local", "scope": "instance"} }

// captureLoadedScopes records every scope handed to the service loaders (plain and with config).
func captureLoadedScopes(t *testing.T, svc *fakeSecretService) *[]secretScope {
	t.Helper()

	scopes := &[]secretScope{}
	origLoad, origLoadCfg := loadServiceFn, loadServiceAndConfigFn
	loadServiceFn = func(scope secretScope) (secretService, error) {
		*scopes = append(*scopes, scope)
		return svc, nil
	}
	loadServiceAndConfigFn = func(scope secretScope) (secretService, *schema.AtmosConfiguration, error) {
		*scopes = append(*scopes, scope)
		return svc, &schema.AtmosConfiguration{}, nil
	}
	t.Cleanup(func() { loadServiceFn, loadServiceAndConfigFn = origLoad, origLoadCfg })
	return scopes
}

func stackScopedService() *fakeSecretService {
	svc := newFakeSecretService()
	svc.declarations = []secrets.Declaration{{Name: "TOKEN"}}
	svc.declared = map[string]bool{"TOKEN": true}
	svc.getValues["TOKEN"] = "value"
	svc.scopes = map[string]secrets.Scope{"TOKEN": secrets.ScopeStack}
	return svc
}

// Every command that operates on declared secrets works with --stack alone for stack-scoped
// secrets, loading the service restricted to the shared declarations.
func TestStackScope_CommandsWithoutComponent(t *testing.T) {
	envFile := func(t *testing.T) string { return writeEnvFile(t, "TOKEN=pushed\n") }
	tests := []struct {
		name   string
		args   func(t *testing.T) []string
		verify func(t *testing.T, svc *fakeSecretService)
	}{
		{
			name: "get",
			args: func(*testing.T) []string { return []string{"get", "TOKEN", "-s", "dev"} },
			verify: func(t *testing.T, svc *fakeSecretService) {
				require.Len(t, svc.getCalls, 1)
				assert.Equal(t, "TOKEN", svc.getCalls[0].name)
			},
		},
		{
			name: "delete by name through the unset alias",
			args: func(*testing.T) []string { return []string{"unset", "TOKEN", "-s", "dev", "--force"} },
			verify: func(t *testing.T, svc *fakeSecretService) {
				assert.Equal(t, []string{"TOKEN"}, svc.deletedNames)
			},
		},
		{
			name: "delete --all",
			args: func(*testing.T) []string { return []string{"delete", "--all", "-s", "dev", "--force"} },
			verify: func(t *testing.T, svc *fakeSecretService) {
				assert.Equal(t, 1, svc.deleteAllCalls)
			},
		},
		{
			name: "pull",
			args: func(*testing.T) []string { return []string{"pull", "-s", "dev"} },
			verify: func(t *testing.T, svc *fakeSecretService) {
				require.Len(t, svc.getCalls, 1)
				assert.Equal(t, "TOKEN", svc.getCalls[0].name)
			},
		},
		{
			name: "push",
			args: func(t *testing.T) []string { return []string{"push", "--input", envFile(t), "-s", "dev"} },
			verify: func(t *testing.T, svc *fakeSecretService) {
				require.Len(t, svc.setCalls, 1)
				assert.Equal(t, "TOKEN", svc.setCalls[0].name)
				assert.Equal(t, "pushed", svc.setCalls[0].value)
			},
		},
		{
			name: "import file",
			args: func(t *testing.T) []string { return []string{"import", envFile(t), "-s", "dev"} },
			verify: func(t *testing.T, svc *fakeSecretService) {
				require.Len(t, svc.setCalls, 1)
				assert.Equal(t, "TOKEN", svc.setCalls[0].name)
			},
		},
		{
			name: "import from store",
			args: func(*testing.T) []string {
				return []string{"import", "TOKEN", "--from-store", "legacy", "--from-key", "old", "-s", "dev"}
			},
			verify: func(t *testing.T, svc *fakeSecretService) {
				require.Len(t, svc.importCalls, 1)
				assert.Equal(t, "TOKEN", svc.importCalls[0].name)
				assert.Equal(t, "legacy", svc.importCalls[0].src.Store)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupIO(t)
			svc := stackScopedService()
			scopes := captureLoadedScopes(t, svc)
			overrideEnumerateScopes(t, []scopeEntry{sharedEntry("vpc", map[string]map[string]any{"TOKEN": stackSpec()})}, nil)

			require.NoError(t, runSecretSubcommand(t, tt.args(t)...))

			require.Len(t, *scopes, 1)
			assert.Equal(t, secretScope{Stack: "dev", Component: "vpc", ComponentType: "terraform", SharedOnly: true}, (*scopes)[0])
			tt.verify(t, svc)
		})
	}
}

func TestStackScope_ExecWithoutComponent(t *testing.T) {
	setupIO(t)
	svc := stackScopedService()
	scopes := captureLoadedScopes(t, svc)
	overrideEnumerateScopes(t, []scopeEntry{sharedEntry("vpc", map[string]map[string]any{"TOKEN": stackSpec()})}, nil)
	gotArgs, gotEnv := overrideRunCommand(t, nil)

	require.NoError(t, runSecretSubcommand(t, "exec", "-s", "dev", "--", "env"))

	assert.Equal(t, []string{"env"}, *gotArgs)
	assert.Contains(t, *gotEnv, "TOKEN=value")
	require.Len(t, *scopes, 1)
	assert.True(t, (*scopes)[0].SharedOnly)
}

func TestStackScope_ShellWithoutComponent(t *testing.T) {
	setupIO(t)
	svc := stackScopedService()
	scopes := captureLoadedScopes(t, svc)
	overrideEnumerateScopes(t, []scopeEntry{sharedEntry("vpc", map[string]map[string]any{"TOKEN": stackSpec()})}, nil)
	_, _, gotEnv := overrideStartShell(t, nil)

	require.NoError(t, runSecretSubcommand(t, "shell", "-s", "dev"))

	assert.Contains(t, *gotEnv, "TOKEN=value")
	require.Len(t, *scopes, 1)
	assert.True(t, (*scopes)[0].SharedOnly)
}

// An explicit --component never enumerates the stack and keeps the full (instance) declaration set.
func TestStackScope_ExplicitComponentKeepsInstanceScope(t *testing.T) {
	for _, args := range [][]string{
		{"get", "TOKEN", "-s", "dev", "-c", "vpc"},
		{"delete", "TOKEN", "-s", "dev", "-c", "vpc", "--force"},
		{"pull", "-s", "dev", "-c", "vpc"},
	} {
		t.Run(args[0], func(t *testing.T) {
			setupIO(t)
			svc := stackScopedService()
			scopes := captureLoadedScopes(t, svc)
			overrideEnumerateScopes(t, nil, errors.New("must not enumerate"))

			require.NoError(t, runSecretSubcommand(t, args...))

			require.Len(t, *scopes, 1)
			assert.Equal(t, secretScope{Stack: "dev", Component: "vpc"}, (*scopes)[0])
		})
	}
}

// Without a component, a command that cannot prove the secrets are shared asks for one instead of
// guessing. Non-interactive runs get the required-flag error.
func TestStackScope_ComponentStillRequired(t *testing.T) {
	instanceOnly := []scopeEntry{sharedEntry("vpc", map[string]map[string]any{"PRIVATE": instanceSpec()})}
	differing := []scopeEntry{
		sharedEntry("a", map[string]map[string]any{"TOKEN": stackSpec()}),
		sharedEntry("b", map[string]map[string]any{"TOKEN": globalSpec()}),
	}
	tests := []struct {
		name    string
		args    []string
		entries []scopeEntry
		err     error
	}{
		{name: "get instance secret", args: []string{"get", "PRIVATE", "-s", "dev"}, entries: instanceOnly},
		{name: "delete instance secret", args: []string{"delete", "PRIVATE", "-s", "dev", "--force"}, entries: instanceOnly},
		{name: "import instance secret from store", args: []string{"import", "PRIVATE", "--from-store", "legacy", "-s", "dev"}, entries: instanceOnly},
		{name: "get undeclared secret", args: []string{"get", "MISSING", "-s", "dev"}, entries: instanceOnly},
		{name: "get differing declarations", args: []string{"get", "TOKEN", "-s", "dev"}, entries: differing},
		{name: "pull with only instance secrets", args: []string{"pull", "-s", "dev"}, entries: instanceOnly},
		{name: "delete --all with only instance secrets", args: []string{"delete", "--all", "-s", "dev", "--force"}, entries: instanceOnly},
		{name: "exec with differing declarations", args: []string{"exec", "-s", "dev", "--", "env"}, entries: differing},
		{name: "shell with no secrets", args: []string{"shell", "-s", "dev"}, entries: nil},
		{name: "push with enumeration failure", args: []string{"push", "--input", "unused", "-s", "dev"}, err: errors.New("describe failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupIO(t)
			svc := stackScopedService()
			scopes := captureLoadedScopes(t, svc)
			overrideEnumerateScopes(t, tt.entries, tt.err)

			err := runSecretSubcommand(t, tt.args...)

			require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
			assert.Empty(t, *scopes, "no backend may be loaded without a component")
			assert.Empty(t, svc.deletedNames)
			assert.Zero(t, svc.deleteAllCalls)
			assert.Empty(t, svc.setCalls)
		})
	}
}

func TestFindSharedStackContext(t *testing.T) {
	tests := []struct {
		name          string
		entries       []scopeEntry
		wantComponent string
		wantErr       bool
	}{
		{
			name: "instance declarations are ignored",
			entries: []scopeEntry{sharedEntry("vpc", map[string]map[string]any{
				"TOKEN": stackSpec(), "PRIVATE": instanceSpec(),
			})},
			wantComponent: "vpc",
		},
		{
			name: "chooses the component rendering every shared declaration",
			entries: []scopeEntry{
				sharedEntry("a", map[string]map[string]any{"TOKEN": stackSpec()}),
				sharedEntry("b", map[string]map[string]any{"TOKEN": stackSpec(), "EXTRA": globalSpec()}),
			},
			wantComponent: "b",
		},
		{
			name: "no component renders every shared declaration",
			entries: []scopeEntry{
				sharedEntry("a", map[string]map[string]any{"TOKEN": stackSpec()}),
				sharedEntry("b", map[string]map[string]any{"EXTRA": globalSpec()}),
			},
			wantErr: true,
		},
		{
			name: "providers differ between components",
			entries: []scopeEntry{
				sharedEntry("a", map[string]map[string]any{"TOKEN": stackSpec()}),
				func() scopeEntry {
					e := sharedEntry("b", map[string]map[string]any{"TOKEN": stackSpec()})
					e.Section["secrets"].(map[string]any)["providers"] = map[string]any{"local": map[string]any{"kind": "sops/age"}}
					return e
				}(),
			},
			wantErr: true,
		},
		{
			name:    "other stacks are ignored",
			entries: []scopeEntry{{Stack: "prod", Component: "vpc", Section: secretDeclarationSection("TOKEN", stackSpec())}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideEnumerateScopes(t, tt.entries, nil)

			component, _, err := findSharedStackContext(secretScope{Stack: "dev"}, "pull")

			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantComponent, component)
		})
	}
}

func TestDescribeScope(t *testing.T) {
	assert.Equal(t, "component `vpc` in stack `dev`", describeScope(secretScope{Stack: "dev", Component: "vpc"}))
	assert.Contains(t, describeScope(secretScope{Stack: "dev", Component: "vpc", SharedOnly: true}), "stack `dev`")
	assert.NotContains(t, describeScope(secretScope{Stack: "dev", Component: "vpc", SharedOnly: true}), "vpc")
}

// A positional NAME is never silently dropped by --all, which would delete every shared secret.
func TestStackScope_DeleteAllRejectsName(t *testing.T) {
	svc := stackScopedService()
	scopes := captureLoadedScopes(t, svc)
	overrideEnumerateScopes(t, []scopeEntry{sharedEntry("vpc", map[string]map[string]any{"TOKEN": stackSpec()})}, nil)

	err := runSecretSubcommand(t, "delete", "TOKEN", "--all", "-s", "dev", "--force")

	require.ErrorIs(t, err, errUtils.ErrMutuallyExclusiveFlags)
	assert.Empty(t, *scopes)
	assert.Zero(t, svc.deleteAllCalls)
	assert.Empty(t, svc.deletedNames)
}
