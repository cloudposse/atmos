package secret

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
