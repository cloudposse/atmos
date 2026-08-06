package secret

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/secrets"
)

func TestResolveSetValue_Inline(t *testing.T) {
	got, err := resolveSetValue("inline-val", true, false)
	require.NoError(t, err)
	assert.Equal(t, "inline-val", got)
}

func TestResolveSetValue_Prompt(t *testing.T) {
	overridePromptForValue(t, "from-prompt", nil)
	got, err := resolveSetValue("", false, false)
	require.NoError(t, err)
	assert.Equal(t, "from-prompt", got)
}

func TestResolveSetValue_Stdin(t *testing.T) {
	// Replace os.Stdin with a pipe so resolveSetValue reads the value we write (cross-platform).
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		_ = r.Close()
		os.Stdin = orig
	})

	// A trailing newline must be trimmed.
	_, writeErr := w.WriteString("piped-secret\n")
	require.NoError(t, writeErr)
	require.NoError(t, w.Close())

	got, err := resolveSetValue("", false, true)
	require.NoError(t, err)
	assert.Equal(t, "piped-secret", got)
}

func TestRunSecretSet_Inline(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "set", "API_KEY=v1", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	require.Len(t, svc.setCalls, 1)
	assert.Equal(t, "API_KEY", svc.setCalls[0].name)
	assert.Equal(t, "v1", svc.setCalls[0].value)
}

// TestRunSecretSet_SharedScopes proves a component context can set inherited stack/global
// declarations; the service resolves the backend coordinate from the declaration scope.
func TestRunSecretSet_SharedScopes(t *testing.T) {
	svc := newFakeSecretService()
	svc.scopes = map[string]secrets.Scope{"SHARED": secrets.ScopeStack}
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "set", "SHARED=v1", "--stack", "dev", "--component", "api")
	require.NoError(t, err)
	require.Len(t, svc.setCalls, 1)
	assert.Equal(t, "SHARED", svc.setCalls[0].name)

	svc2 := newFakeSecretService()
	svc2.scopes = map[string]secrets.Scope{"GLOBAL": secrets.ScopeGlobal}
	installService(t, svc2, nil)

	err = runSecretSubcommand(t, "set", "GLOBAL=v2", "--stack", "dev", "--component", "api")
	require.NoError(t, err)
	require.Len(t, svc2.setCalls, 1)
	assert.Equal(t, "GLOBAL", svc2.setCalls[0].name)
}

func TestRunSecretSet_GlobalScopeWithoutComponent(t *testing.T) {
	svc := newFakeSecretService()
	svc.scopes = map[string]secrets.Scope{"SHARED_TOKEN": secrets.ScopeGlobal}
	installService(t, svc, nil)
	overrideEnumerateScopes(t, []scopeEntry{
		{
			Stack:         "dev",
			Component:     "example-service",
			ComponentType: "helm",
			Section: secretDeclarationSection("SHARED_TOKEN", map[string]any{
				"store": "example-secrets",
				"scope": "global",
			}),
		},
	}, nil)

	err := runSecretSubcommand(t, "set", "SHARED_TOKEN=v1", "--stack", "dev")
	require.NoError(t, err)
	require.Len(t, svc.setCalls, 1)
	assert.Equal(t, "SHARED_TOKEN", svc.setCalls[0].name)
	assert.Equal(t, "v1", svc.setCalls[0].value)
}

func TestRunSecretSet_GlobalScopeWithoutComponentPreservesType(t *testing.T) {
	svc := newFakeSecretService()
	svc.scopes = map[string]secrets.Scope{"SHARED_TOKEN": secrets.ScopeGlobal}
	installService(t, svc, nil)
	originalLoadService := loadServiceFn
	var loadedScope secretScope
	loadServiceFn = func(scope secretScope) (secretService, error) {
		loadedScope = scope
		return originalLoadService(scope)
	}
	t.Cleanup(func() { loadServiceFn = originalLoadService })
	overrideEnumerateScopes(t, []scopeEntry{
		{
			Stack:         "dev",
			Component:     "example-service",
			ComponentType: "helm",
			Section: secretDeclarationSection("SHARED_TOKEN", map[string]any{
				"store": "example-secrets",
				"scope": "global",
			}),
		},
	}, nil)

	err := runSecretSubcommand(t, "set", "SHARED_TOKEN=v1", "--stack", "dev", "--type", "helm")
	require.NoError(t, err)
	require.Len(t, svc.setCalls, 1)
	assert.Equal(t, "helm", loadedScope.ComponentType)
}

func TestFindGlobalSetContext(t *testing.T) {
	sharedSection := secretDeclarationSection("SHARED_TOKEN", map[string]any{"store": "example-secrets", "scope": "global"})
	tests := []struct {
		name              string
		entries           []scopeEntry
		enumerationErr    error
		scope             secretScope
		expectedComponent string
		expectedType      string
		expectedErr       error
	}{
		{
			name:           "enumeration error",
			enumerationErr: errors.New("stack enumeration failed"),
			scope:          secretScope{Stack: "dev"},
			expectedErr:    errUtils.ErrRequiredFlagNotProvided,
		},
		{
			name: "no matching declaration",
			entries: []scopeEntry{
				{
					Stack:         "prod",
					Component:     "other-stack-service",
					ComponentType: "helm",
					Section:       secretDeclarationSection("SHARED_TOKEN", map[string]any{"store": "example-secrets", "scope": "global"}),
				},
				{
					Stack:         "dev",
					Component:     "other-type-service",
					ComponentType: "terraform",
					Section:       secretDeclarationSection("SHARED_TOKEN", map[string]any{"store": "example-secrets", "scope": "global"}),
				},
				{
					Stack:         "dev",
					Component:     "example-service",
					ComponentType: "helm",
					Section:       secretDeclarationSection("OTHER_TOKEN", map[string]any{"store": "example-secrets", "scope": "global"}),
				},
			},
			scope:       secretScope{Stack: "dev", ComponentType: "helm"},
			expectedErr: errUtils.ErrRequiredFlagNotProvided,
		},
		{
			name: "inconsistent declarations",
			entries: []scopeEntry{
				{
					Stack:         "dev",
					Component:     "example-service-a",
					ComponentType: "helm",
					Section:       secretDeclarationSection("SHARED_TOKEN", map[string]any{"store": "example-secrets-a", "scope": "global"}),
				},
				{
					Stack:         "dev",
					Component:     "example-service-b",
					ComponentType: "helm",
					Section:       secretDeclarationSection("SHARED_TOKEN", map[string]any{"store": "example-secrets-b", "scope": "global"}),
				},
			},
			scope:       secretScope{Stack: "dev"},
			expectedErr: errUtils.ErrRequiredFlagNotProvided,
		},
		{
			name: "identical declarations select first component",
			entries: []scopeEntry{
				{Stack: "dev", Component: "example-service-a", ComponentType: "helm", Section: sharedSection},
				{Stack: "dev", Component: "example-service-b", ComponentType: "helm", Section: sharedSection},
			},
			scope:             secretScope{Stack: "dev"},
			expectedComponent: "example-service-a",
			expectedType:      "helm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overrideEnumerateScopes(t, tt.entries, tt.enumerationErr)

			component, componentType, err := findGlobalSetContext(tt.scope, "SHARED_TOKEN")
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedComponent, component)
			assert.Equal(t, tt.expectedType, componentType)
		})
	}
}

func TestRunSecretSet_NonGlobalScopeStillRequiresComponent(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)
	overrideEnumerateScopes(t, []scopeEntry{
		{
			Stack:         "dev",
			Component:     "example-service",
			ComponentType: "helm",
			Section: secretDeclarationSection("API_KEY", map[string]any{
				"store": "example-secrets",
				"scope": "instance",
			}),
		},
	}, nil)

	err := runSecretSubcommand(t, "set", "API_KEY=v1", "--stack", "dev")
	require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
	assert.Empty(t, svc.setCalls)
}

func secretDeclarationSection(name string, spec map[string]any) map[string]any {
	return map[string]any{
		"secrets": map[string]any{
			"vars": map[string]any{name: spec},
		},
	}
}

func TestRunSecretSet_Prompt(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)
	overridePromptForValue(t, "prompted-secret", nil)

	// No inline value and no --stdin → the prompt path is used.
	err := runSecretSubcommand(t, "set", "API_KEY", "--stack", "dev", "--component", "api")
	require.NoError(t, err)

	require.Len(t, svc.setCalls, 1)
	assert.Equal(t, "API_KEY", svc.setCalls[0].name)
	assert.Equal(t, "prompted-secret", svc.setCalls[0].value)
}

func TestRunSecretSet_PromptError(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)
	sentinel := errors.New("prompt aborted")
	overridePromptForValue(t, "", sentinel)

	err := runSecretSubcommand(t, "set", "API_KEY", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, sentinel)
	assert.Empty(t, svc.setCalls)
}

func TestRunSecretSet_EmptyName(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	// "=v1" cuts to an empty name.
	err := runSecretSubcommand(t, "set", "=v1", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
	assert.Empty(t, svc.setCalls)
}

func TestRunSecretSet_EmptyNameWithoutComponent(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "set", "=v1", "--stack", "dev")
	require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
	assert.Empty(t, svc.setCalls)
}

func TestRunSecretSet_SetError(t *testing.T) {
	svc := newFakeSecretService()
	svc.setErr = errors.New("backend write failed")
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "set", "API_KEY=v1", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, svc.setErr)
	require.Len(t, svc.setCalls, 1)
}

func TestRunSecretSet_MissingScope(t *testing.T) {
	svc := newFakeSecretService()
	installService(t, svc, nil)

	// No --stack/--component → parseScope rejects before loading the service.
	err := runSecretSubcommand(t, "set", "API_KEY=v1")
	require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
	assert.Empty(t, svc.setCalls)
}

func TestRunSecretSet_LoadServiceError(t *testing.T) {
	loadErr := errors.New("failed to load service")
	installService(t, nil, loadErr)

	err := runSecretSubcommand(t, "set", "API_KEY=v1", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, loadErr)
}
