package secret

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/secrets"
)

// TestInitWholeStack_DedupesStackScoped proves whole-stack init provisions each instance's
// instance-scoped secret while a stack-scoped secret is prompted/set only once across components.
func TestInitWholeStack_DedupesStackScoped(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	svc.statuses = []secrets.Status{
		{Declaration: secrets.Declaration{Name: "SHARED", Scope: secrets.ScopeStack}},
		{Declaration: secrets.Declaration{Name: "INST", Scope: secrets.ScopeInstance}},
	}
	installService(t, svc, nil)
	overrideEnumerateScopes(t, []scopeEntry{
		{Stack: "prod", Component: "api"},
		{Stack: "prod", Component: "web"},
	}, nil)
	overridePromptForValue(t, "value", nil)

	err := initWholeStack(secretScope{Stack: "prod"}, initOptions{mode: "warn"})
	require.NoError(t, err)

	names := make([]string, 0, len(svc.setCalls))
	for _, c := range svc.setCalls {
		names = append(names, c.name)
	}
	assert.ElementsMatch(t, []string{"SHARED", "INST", "INST"}, names,
		"stack-scoped SHARED set once; instance-scoped INST set per component")
}

// TestInitWholeStack_DryRun exercises the dry-run reporting path (no prompts, no writes).
func TestInitWholeStack_DryRun(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	svc.statuses = []secrets.Status{
		{Declaration: secrets.Declaration{Name: "INST", Scope: secrets.ScopeInstance}, Initialized: true},
	}
	installService(t, svc, nil)
	overrideEnumerateScopes(t, []scopeEntry{{Stack: "prod", Component: "api"}}, nil)

	require.NoError(t, initWholeStack(secretScope{Stack: "prod"}, initOptions{dryRun: true, mode: "warn"}))
	assert.Empty(t, svc.setCalls, "dry-run must not write")
}

// TestInitWholeStack_NoEntries prints the empty message and succeeds.
func TestInitWholeStack_NoEntries(t *testing.T) {
	setupIO(t)
	overrideEnumerateScopes(t, nil, nil)

	require.NoError(t, initWholeStack(secretScope{Stack: "prod"}, initOptions{mode: "warn"}))
}

// TestInitWholeStack_EnumerateError propagates the enumeration error.
func TestInitWholeStack_EnumerateError(t *testing.T) {
	setupIO(t)
	sentinel := errors.New("enumerate failed")
	overrideEnumerateScopes(t, nil, sentinel)

	require.ErrorIs(t, initWholeStack(secretScope{Stack: "prod"}, initOptions{mode: "warn"}), sentinel)
}

// TestInitWholeStack_LoadServiceError propagates a per-instance service-load failure.
func TestInitWholeStack_LoadServiceError(t *testing.T) {
	setupIO(t)
	loadErr := errors.New("load failed")
	installService(t, nil, loadErr)
	overrideEnumerateScopes(t, []scopeEntry{{Stack: "prod", Component: "api"}}, nil)

	require.ErrorIs(t, initWholeStack(secretScope{Stack: "prod"}, initOptions{mode: "warn"}), loadErr)
}

func TestInitAllScopes_ProvisionsEveryStack(t *testing.T) {
	setupIO(t)
	svc := newFakeSecretService()
	svc.statuses = []secrets.Status{{Declaration: secrets.Declaration{Name: "API_KEY"}}}
	svc.declared["API_KEY"] = true
	installService(t, svc, nil)
	overrideEnumerateScopes(t, []scopeEntry{
		{Stack: "dev", Component: "api"},
		{Stack: "prod", Component: "api"},
	}, nil)

	require.NoError(t, initAllScopes(secretScope{}, initOptions{values: map[string]string{"API_KEY": "value"}, mode: "warn"}))
	require.Len(t, svc.setCalls, 2)
	assert.Equal(t, "API_KEY", svc.setCalls[0].name)
	assert.Equal(t, "API_KEY", svc.setCalls[1].name)
}

// TestInitVerb covers the dry-run verb selection.
func TestInitVerb(t *testing.T) {
	assert.Equal(t, "rotate", initVerb(true))
	assert.Equal(t, "initialize", initVerb(false))
}
