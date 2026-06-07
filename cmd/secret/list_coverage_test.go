package secret

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/secrets"
)

// listScopeSection builds a component section declaring the given secrets, where each entry maps a
// name to its scope ("stack" or "" for instance). Store-backed so they need no provider to enumerate.
func listScopeSection(scopes map[string]string) map[string]any {
	vars := map[string]any{}
	for name, scope := range scopes {
		spec := map[string]any{"store": "app-secrets"}
		if scope != "" {
			spec["scope"] = scope
		}
		vars[name] = spec
	}
	return map[string]any{"secrets": map[string]any{"vars": vars}}
}

// TestEnumeratedSecretRows builds list rows across every (stack, component) instance and proves a
// stack-scoped secret is de-duplicated to one `*`-component row while instance secrets keep theirs.
func TestEnumeratedSecretRows(t *testing.T) {
	overrideEnumerateScopes(t, []scopeEntry{
		{Stack: "prod", Component: "api", Section: listScopeSection(map[string]string{"SHARED": "stack", "API_KEY": ""})},
		{Stack: "prod", Component: "web", Section: listScopeSection(map[string]string{"SHARED": "stack", "WEB_KEY": ""})},
	}, nil)

	rows, err := enumeratedSecretRows(secretScope{})
	require.NoError(t, err)
	require.Len(t, rows, 3, "SHARED de-duplicated to one row; API_KEY and WEB_KEY each keep theirs")

	starRows := 0
	secretsSeen := map[string]bool{}
	for _, row := range rows {
		secretsSeen[row["secret"].(string)] = true
		if row["component"] == "*" {
			starRows++
		}
	}
	assert.Equal(t, 1, starRows, "exactly one stack-scoped `*` row")
	assert.True(t, secretsSeen["SHARED"])
	assert.True(t, secretsSeen["API_KEY"])
	assert.True(t, secretsSeen["WEB_KEY"])
}

// TestEnumeratedSecretRows_Error proves the enumeration error is propagated.
func TestEnumeratedSecretRows_Error(t *testing.T) {
	sentinel := errors.New("enumerate failed")
	overrideEnumerateScopes(t, nil, sentinel)

	_, err := enumeratedSecretRows(secretScope{})
	require.ErrorIs(t, err, sentinel)
}

// TestEmptyListMessage covers the facet-scoped "nothing found" messages.
func TestEmptyListMessage(t *testing.T) {
	assert.Contains(t, emptyListMessage(secretScope{Stack: "prod"}), `stack "prod"`)
	assert.Contains(t, emptyListMessage(secretScope{Component: "api"}), `component "api"`)
	assert.Contains(t, emptyListMessage(secretScope{}), "any stack")
}

// TestRunSecretList_SingleScope drives runSecretList's fast path (both facets set): it loads the
// scoped service, converts its statuses to rows, and renders them — covering statusRow and the
// scope/backend/status label helpers.
func TestRunSecretList_SingleScope(t *testing.T) {
	setupIO(t)

	svc := newFakeSecretService()
	svc.statuses = []secrets.Status{
		{Declaration: secrets.Declaration{Name: "API_KEY", Scope: secrets.ScopeInstance}, Initialized: true},
		{Declaration: secrets.Declaration{Name: "SHARED", Scope: secrets.ScopeStack}, Initialized: false},
	}
	installService(t, svc, nil)

	err := runSecretSubcommand(t, "list", "--stack", "dev", "--component", "api")
	require.NoError(t, err)
}

// TestRunSecretList_SingleScope_LoadError proves a service-load failure on the fast path is
// returned to the caller.
func TestRunSecretList_SingleScope_LoadError(t *testing.T) {
	setupIO(t)

	sentinel := errors.New("load failed")
	installService(t, newFakeSecretService(), sentinel)

	err := runSecretSubcommand(t, "list", "--stack", "dev", "--component", "api")
	require.ErrorIs(t, err, sentinel)
}

// TestRunSecretList_Enumerated drives the enumerated path (facets not fully specified): it walks
// every declaring instance via the enumerate seam and renders the resulting rows.
func TestRunSecretList_Enumerated(t *testing.T) {
	setupIO(t)

	overrideEnumerateScopes(t, []scopeEntry{
		{Stack: "prod", Component: "api", Section: listScopeSection(map[string]string{"API_KEY": ""})},
	}, nil)

	err := runSecretSubcommand(t, "list", "--stack", "prod")
	require.NoError(t, err)
}
