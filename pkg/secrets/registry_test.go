package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractDeclarations(t *testing.T) {
	section := map[string]any{
		"secrets": map[string]any{
			"vars": map[string]any{
				"DATADOG_API_KEY": map[string]any{
					"description": "Datadog API key",
					"store":       "app-secrets",
					"required":    true,
				},
				"GITHUB_APP_KEY": map[string]any{
					"sops": "dev-sops",
				},
				"NO_BACKEND": map[string]any{
					"description": "missing backend",
				},
			},
		},
	}

	decls := ExtractDeclarations(section)
	require.Len(t, decls, 3)

	dd := decls["DATADOG_API_KEY"]
	assert.Equal(t, BackendStore, dd.BackendType)
	assert.Equal(t, "app-secrets", dd.BackendName)
	assert.True(t, dd.Required)
	assert.Equal(t, "Datadog API key", dd.Description)

	gh := decls["GITHUB_APP_KEY"]
	assert.Equal(t, BackendSops, gh.BackendType)
	assert.Equal(t, "dev-sops", gh.BackendName)
	assert.False(t, gh.Required)

	nb := decls["NO_BACKEND"]
	assert.Equal(t, BackendType(""), nb.BackendType)
	assert.Empty(t, nb.BackendName)
}

func TestExtractDeclarations_Empty(t *testing.T) {
	assert.Empty(t, ExtractDeclarations(nil))
	assert.Empty(t, ExtractDeclarations(map[string]any{}))
	assert.Empty(t, ExtractDeclarations(map[string]any{"secrets": map[string]any{}}))
}

func TestLookupDeclaration(t *testing.T) {
	section := map[string]any{
		"secrets": map[string]any{
			"vars": map[string]any{
				"API_KEY": map[string]any{"store": "s"},
			},
		},
	}
	decl, ok := LookupDeclaration(section, "API_KEY")
	require.True(t, ok)
	assert.Equal(t, "API_KEY", decl.Name)

	_, ok = LookupDeclaration(section, "MISSING")
	assert.False(t, ok)
}
