package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCoordinateForDeclaration_Scope proves a stack-scoped declaration drops the component segment
// (one shared value per stack) while an instance-scoped declaration keeps it.
func TestCoordinateForDeclaration_Scope(t *testing.T) {
	stackDecl := &Declaration{Name: "DATADOG_API_KEY", Scope: ScopeStack}
	coord := coordinateForDeclaration(stackDecl, "prod", "api")
	assert.Equal(t, "prod", coord.Stack)
	assert.Empty(t, coord.Component, "stack-scoped coordinate must omit the component")
	assert.Equal(t, ScopeStack, coord.Scope)

	instDecl := &Declaration{Name: "DATADOG_API_KEY", Scope: ScopeInstance}
	coord = coordinateForDeclaration(instDecl, "prod", "api")
	assert.Equal(t, "api", coord.Component, "instance-scoped coordinate must keep the component")
	assert.Equal(t, ScopeInstance, coord.Scope)

	// An empty scope defaults to instance (back-compat).
	defDecl := &Declaration{Name: "X"}
	coord = coordinateForDeclaration(defDecl, "prod", "api")
	assert.Equal(t, "api", coord.Component)
	assert.Equal(t, ScopeInstance, coord.Scope)

	// A global declaration drops both the stack and component segments (one shared value per
	// backend), regardless of the resolving scope.
	globalDecl := &Declaration{Name: "SHARED_CLIENT_SECRET", Scope: ScopeGlobal}
	coord = coordinateForDeclaration(globalDecl, "prod", "api")
	assert.Empty(t, coord.Stack, "global coordinate must omit the stack")
	assert.Empty(t, coord.Component, "global coordinate must omit the component")
	assert.Equal(t, "SHARED_CLIENT_SECRET", coord.Key)
	assert.Equal(t, ScopeGlobal, coord.Scope)

	coordOther := coordinateForDeclaration(globalDecl, "dev", "web")
	assert.Equal(t, coord, coordOther, "every resolving scope must converge on the same global coordinate")
}

// TestTagScope_StampsAndResolvesOverride proves position-derived scope tagging plus the standard
// deep-merge semantics: a component-level (instance) tag overrides an inherited stack-level (stack)
// tag, and a stack-only secret keeps stack scope. It also proves the input is not mutated.
func TestTagScope_StampsAndResolvesOverride(t *testing.T) {
	global := map[string]any{
		"vars": map[string]any{
			"SHARED":     map[string]any{"sops": "default"},
			"STACK_ONLY": map[string]any{"sops": "default"},
		},
	}
	component := map[string]any{
		"vars": map[string]any{
			"SHARED": map[string]any{"sops": "default"}, // re-declared at instance level => override
		},
	}

	taggedGlobal, err := TagScope(global, ScopeStack)
	require.NoError(t, err)
	taggedComponent, err := TagScope(component, ScopeInstance)
	require.NoError(t, err)

	// Input not mutated.
	_, hasScope := global["vars"].(map[string]any)["SHARED"].(map[string]any)["scope"]
	assert.False(t, hasScope, "TagScope must not mutate the input section")

	// Stack layer stamped stack; component layer stamped instance.
	assert.Equal(t, "stack", taggedGlobal["vars"].(map[string]any)["STACK_ONLY"].(map[string]any)["scope"])
	assert.Equal(t, "instance", taggedComponent["vars"].(map[string]any)["SHARED"].(map[string]any)["scope"])

	// After the deep-merge "most-specific wins", the merged section resolves SHARED to instance and
	// STACK_ONLY to stack. Simulate the merge by reading both through ExtractDeclarations.
	mergedVars := map[string]any{}
	for k, v := range taggedGlobal["vars"].(map[string]any) {
		mergedVars[k] = v
	}
	for k, v := range taggedComponent["vars"].(map[string]any) {
		mergedVars[k] = v // component overrides global for the same key
	}
	decls := ExtractDeclarations(map[string]any{"secrets": map[string]any{"vars": mergedVars}})
	assert.Equal(t, ScopeInstance, decls["SHARED"].Scope, "re-declared secret must resolve to instance scope")
	assert.Equal(t, ScopeStack, decls["STACK_ONLY"].Scope, "stack-only secret must remain stack-scoped")
}

// TestTagScope_ConflictRejected proves an explicit scope that conflicts with position errors (the
// one-way rule: a component-level declaration can't claim stack scope).
func TestTagScope_ConflictRejected(t *testing.T) {
	section := map[string]any{
		"vars": map[string]any{
			"X": map[string]any{"sops": "default", "scope": "stack"},
		},
	}
	_, err := TagScope(section, ScopeInstance)
	require.ErrorIs(t, err, ErrScopeConflict)
}

// TestTagScope_GlobalSurvivesEitherPosition proves an explicit `scope: global` is exempt from the
// positional-conflict rule: it survives the stamp at both the stack-level and component-level
// positions (a global declaration is typically a catalog fragment imported anywhere).
func TestTagScope_GlobalSurvivesEitherPosition(t *testing.T) {
	for _, positional := range []Scope{ScopeStack, ScopeInstance} {
		section := map[string]any{
			"vars": map[string]any{
				"SHARED_CLIENT_SECRET": map[string]any{"store": "app-secrets", "scope": "global"},
			},
		}
		tagged, err := TagScope(section, positional)
		require.NoError(t, err, "explicit global must not conflict with positional scope %q", positional)
		got := tagged["vars"].(map[string]any)["SHARED_CLIENT_SECRET"].(map[string]any)["scope"]
		assert.Equal(t, "global", got, "explicit global must survive the %q positional stamp", positional)
	}
}

// TestExtractDeclarations_ScopeDefault proves a declaration without a scope tag defaults to
// instance, and explicit stack/global tags are honored.
func TestExtractDeclarations_ScopeDefault(t *testing.T) {
	section := map[string]any{
		"secrets": map[string]any{
			"vars": map[string]any{
				"X": map[string]any{"sops": "default"},
				"Y": map[string]any{"sops": "default", "scope": "stack"},
				"Z": map[string]any{"store": "app-secrets", "scope": "global"},
			},
		},
	}
	decls := ExtractDeclarations(section)
	assert.Equal(t, ScopeInstance, decls["X"].Scope)
	assert.Equal(t, ScopeStack, decls["Y"].Scope)
	assert.Equal(t, ScopeGlobal, decls["Z"].Scope)
}
