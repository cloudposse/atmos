package list

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRenderGraphviz_Empty(t *testing.T) {
	output, err := RenderGraphviz(nil, nil, nil)
	require.NoError(t, err)

	assert.Contains(t, output, "digraph AuthConfig")
	assert.Contains(t, output, "No providers or identities configured")
}

func TestRenderGraphviz_ProvidersOnly(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:    "aws/iam-identity-center",
			Default: true,
		},
		"okta": {
			Kind: "okta",
		},
	}

	output, err := RenderGraphviz(nil, providers, nil)
	require.NoError(t, err)

	assert.Contains(t, output, "digraph AuthConfig")
	assert.Contains(t, output, "rankdir=LR")
	assert.Contains(t, output, "aws-sso")
	assert.Contains(t, output, "okta")
	assert.Contains(t, output, "fillcolor=lightblue")
}

func TestRenderGraphviz_IdentitiesOnly(t *testing.T) {
	identities := map[string]schema.Identity{
		"admin": {
			Kind:    "aws/permission-set",
			Default: true,
			Via:     &schema.IdentityVia{Provider: "aws-sso"},
		},
	}

	output, err := RenderGraphviz(nil, nil, identities)
	require.NoError(t, err)

	assert.Contains(t, output, "digraph AuthConfig")
	assert.Contains(t, output, "admin")
	assert.Contains(t, output, "fillcolor=lightgreen")
	assert.Contains(t, output, "aws-sso\" -> \"admin")
}

func TestRenderGraphviz_WithEdges(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind: "aws/iam-identity-center",
		},
	}

	identities := map[string]schema.Identity{
		"admin": {
			Kind: "aws/permission-set",
			Via:  &schema.IdentityVia{Provider: "aws-sso"},
		},
		"dev": {
			Kind: "aws/assume-role",
			Via:  &schema.IdentityVia{Identity: "admin"},
		},
	}

	output, err := RenderGraphviz(nil, providers, identities)
	require.NoError(t, err)

	assert.Contains(t, output, "aws-sso\" -> \"admin")
	assert.Contains(t, output, "admin\" -> \"dev")
}

func TestRenderGraphviz_EscapesSpecialCharacters(t *testing.T) {
	providers := map[string]schema.Provider{
		"provider-with-\"quotes\"": {
			Kind: "test\\backslash",
		},
	}

	identities := map[string]schema.Identity{
		"identity\nwith\nnewlines": {
			Kind: "test",
			Via:  &schema.IdentityVia{Provider: "provider-with-\"quotes\""},
		},
	}

	output, err := RenderGraphviz(nil, providers, identities)
	require.NoError(t, err)

	// Verify escaping.
	assert.Contains(t, output, "\\\"")
	assert.Contains(t, output, "\\\\")
	assert.Contains(t, output, "\\n")
}

func TestRenderMermaid_Empty(t *testing.T) {
	output, err := RenderMermaid(nil, nil, nil)
	require.NoError(t, err)

	assert.Contains(t, output, "graph LR")
	assert.Contains(t, output, "Empty[No providers or identities configured]")
}

func TestRenderMermaid_ProvidersOnly(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:    "aws/iam-identity-center",
			Default: true,
		},
		"okta": {
			Kind: "okta",
		},
	}

	output, err := RenderMermaid(nil, providers, nil)
	require.NoError(t, err)

	assert.Contains(t, output, "graph LR")
	assert.Contains(t, output, "aws_sso[")
	assert.Contains(t, output, "okta[")
	assert.Contains(t, output, "class aws_sso provider")
	assert.Contains(t, output, "class aws_sso default")
	assert.Contains(t, output, "class okta provider")
}

func TestRenderMermaid_IdentitiesOnly(t *testing.T) {
	identities := map[string]schema.Identity{
		"admin": {
			Kind:    "aws/permission-set",
			Default: true,
			Via:     &schema.IdentityVia{Provider: "aws-sso"},
		},
	}

	output, err := RenderMermaid(nil, nil, identities)
	require.NoError(t, err)

	assert.Contains(t, output, "graph LR")
	assert.Contains(t, output, "admin[")
	assert.Contains(t, output, "class admin identity")
	assert.Contains(t, output, "class admin default")
	assert.Contains(t, output, "aws_sso --> admin")
}

func TestRenderMermaid_WithChainedIdentities(t *testing.T) {
	identities := map[string]schema.Identity{
		"base": {
			Kind: "aws/permission-set",
			Via:  &schema.IdentityVia{Provider: "aws-sso"},
		},
		"derived": {
			Kind: "aws/assume-role",
			Via:  &schema.IdentityVia{Identity: "base"},
		},
	}

	output, err := RenderMermaid(nil, nil, identities)
	require.NoError(t, err)

	assert.Contains(t, output, "aws_sso --> base")
	assert.Contains(t, output, "base --> derived")
}

func TestRenderMermaid_EscapesSpecialCharacters(t *testing.T) {
	providers := map[string]schema.Provider{
		"provider-with-<tags>": {
			Kind: "test\"quotes",
		},
	}

	output, err := RenderMermaid(nil, providers, nil)
	require.NoError(t, err)

	// Verify HTML entity escaping.
	assert.Contains(t, output, "&lt;")
	assert.Contains(t, output, "&gt;")
	assert.Contains(t, output, "&quot;")
}

func TestRenderMarkdown_Empty(t *testing.T) {
	output, err := RenderMarkdown(nil, nil, nil)
	require.NoError(t, err)

	assert.Contains(t, output, "# Authentication Configuration")
	assert.Contains(t, output, "No providers or identities configured")
}

func TestRenderMarkdown_ProvidersOnly(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:     "aws/iam-identity-center",
			Region:   "us-east-1",
			StartURL: "https://d-abc123.awsapps.com/start",
			Default:  true,
		},
	}

	output, err := RenderMarkdown(nil, providers, nil)
	require.NoError(t, err)

	assert.Contains(t, output, "# Authentication Configuration")
	assert.Contains(t, output, "```mermaid")
	assert.Contains(t, output, "graph LR")
	assert.Contains(t, output, "aws_sso[")
	assert.Contains(t, output, "```")
}

func TestRenderMarkdown_IdentitiesOnly(t *testing.T) {
	identities := map[string]schema.Identity{
		"admin": {
			Kind:    "aws/permission-set",
			Alias:   "administrator",
			Default: true,
			Via:     &schema.IdentityVia{Provider: "aws-sso"},
		},
	}

	output, err := RenderMarkdown(nil, nil, identities)
	require.NoError(t, err)

	assert.Contains(t, output, "# Authentication Configuration")
	assert.Contains(t, output, "```mermaid")
	assert.Contains(t, output, "admin[")
	assert.Contains(t, output, "aws_sso --> admin")
}

func TestRenderMarkdown_WithProviderURL(t *testing.T) {
	providers := map[string]schema.Provider{
		"okta": {
			Kind: "okta",
			URL:  "https://company.okta.com/app",
		},
	}

	output, err := RenderMarkdown(nil, providers, nil)
	require.NoError(t, err)

	assert.Contains(t, output, "```mermaid")
	assert.Contains(t, output, "okta[")
}

func TestRenderMarkdown_WithViaIdentity(t *testing.T) {
	identities := map[string]schema.Identity{
		"admin": {
			Kind: "aws/permission-set",
			Via:  &schema.IdentityVia{Provider: "aws-sso"},
		},
		"dev": {
			Kind: "aws/assume-role",
			Via:  &schema.IdentityVia{Identity: "admin"},
		},
	}

	output, err := RenderMarkdown(nil, nil, identities)
	require.NoError(t, err)

	assert.Contains(t, output, "admin --> dev")
}

func TestRenderMarkdown_WithMermaidDiagram(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind: "aws/iam-identity-center",
		},
	}

	identities := map[string]schema.Identity{
		"admin": {
			Kind: "aws/permission-set",
			Via:  &schema.IdentityVia{Provider: "aws-sso"},
		},
	}

	output, err := RenderMarkdown(nil, providers, identities)
	require.NoError(t, err)

	assert.Contains(t, output, "# Authentication Configuration")
	assert.Contains(t, output, "```mermaid")
	assert.Contains(t, output, "graph LR")
	assert.Contains(t, output, "aws_sso[")
	assert.Contains(t, output, "admin[")
	assert.Contains(t, output, "aws_sso --> admin")
	assert.Contains(t, output, "```")
}

func TestEscapeGraphvizLabel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "backslash",
			input:    "test\\backslash",
			expected: "test\\\\backslash",
		},
		{
			name:     "quotes",
			input:    "test\"quotes\"",
			expected: "test\\\"quotes\\\"",
		},
		{
			name:     "newlines",
			input:    "line1\nline2",
			expected: "line1\\nline2",
		},
		{
			name:     "combined",
			input:    "test\\\"with\nnewline",
			expected: "test\\\\\\\"with\\nnewline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeGraphvizLabel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEscapeGraphvizID(t *testing.T) {
	result := escapeGraphvizID("node\"with\"quotes")
	assert.Equal(t, "node\\\"with\\\"quotes", result)
}

func TestEscapeMermaidLabel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "quotes",
			input:    "test\"quotes\"",
			expected: "test&quot;quotes&quot;",
		},
		{
			name:     "angle brackets",
			input:    "test<tags>",
			expected: "test&lt;tags&gt;",
		},
		{
			name:     "combined",
			input:    "<test>\"value\"",
			expected: "&lt;test&gt;&quot;value&quot;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeMermaidLabel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeMermaidID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "hyphens",
			input:    "aws-sso",
			expected: "aws_sso",
		},
		{
			name:     "dots",
			input:    "provider.name",
			expected: "provider_name",
		},
		{
			name:     "slashes",
			input:    "aws/iam",
			expected: "aws_iam",
		},
		{
			name:     "combined",
			input:    "aws-sso/provider.test",
			expected: "aws_sso_provider_test",
		},
		{
			name:     "no special chars",
			input:    "simple",
			expected: "simple",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeMermaidID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetSortedProviderNames(t *testing.T) {
	providers := map[string]schema.Provider{
		"okta":    {Kind: "okta"},
		"aws-sso": {Kind: "aws"},
		"azure":   {Kind: "azure"},
	}

	names := getSortedProviderNames(providers)

	require.Len(t, names, 3)
	assert.Equal(t, "aws-sso", names[0])
	assert.Equal(t, "azure", names[1])
	assert.Equal(t, "okta", names[2])
}

func TestGetSortedIdentityNames(t *testing.T) {
	identities := map[string]schema.Identity{
		"dev":   {Kind: "dev"},
		"admin": {Kind: "admin"},
		"ci":    {Kind: "ci"},
	}

	names := getSortedIdentityNames(identities)

	require.Len(t, names, 3)
	assert.Equal(t, "admin", names[0])
	assert.Equal(t, "ci", names[1])
	assert.Equal(t, "dev", names[2])
}

// Error condition and edge case tests.

func TestRenderGraphviz_InvalidViaReference(t *testing.T) {
	// Test that referencing non-existent providers doesn't cause errors.
	identities := map[string]schema.Identity{
		"admin": {
			Kind: "aws/permission-set",
			Via:  &schema.IdentityVia{Provider: "nonexistent"},
		},
	}

	output, err := RenderGraphviz(nil, nil, identities)
	require.NoError(t, err)

	// Should still generate valid DOT output with edge to nonexistent provider.
	assert.Contains(t, output, "digraph AuthConfig")
	assert.Contains(t, output, "admin")
	assert.Contains(t, output, "nonexistent\" -> \"admin")
}

func TestRenderGraphviz_CircularIdentityChain(t *testing.T) {
	// Test circular references (identity A -> identity B -> identity A).
	identities := map[string]schema.Identity{
		"identity-a": {
			Kind: "aws/assume-role",
			Via:  &schema.IdentityVia{Identity: "identity-b"},
		},
		"identity-b": {
			Kind: "aws/assume-role",
			Via:  &schema.IdentityVia{Identity: "identity-a"},
		},
	}

	output, err := RenderGraphviz(nil, nil, identities)
	require.NoError(t, err)

	// Should generate valid DOT with circular edges.
	assert.Contains(t, output, "identity-a")
	assert.Contains(t, output, "identity-b")
	assert.Contains(t, output, "identity-b\" -> \"identity-a")
	assert.Contains(t, output, "identity-a\" -> \"identity-b")
}

func TestRenderGraphviz_NilVia(t *testing.T) {
	// Test identity with nil Via.
	identities := map[string]schema.Identity{
		"standalone": {
			Kind: "aws/user",
			Via:  nil,
		},
	}

	output, err := RenderGraphviz(nil, nil, identities)
	require.NoError(t, err)

	assert.Contains(t, output, "standalone")
	assert.Contains(t, output, "aws/user")
}

func TestRenderGraphviz_EmptyStringsInVia(t *testing.T) {
	// Test Via with empty string fields.
	identities := map[string]schema.Identity{
		"identity": {
			Kind: "aws/assume-role",
			Via:  &schema.IdentityVia{Provider: "", Identity: ""},
		},
	}

	output, err := RenderGraphviz(nil, nil, identities)
	require.NoError(t, err)

	assert.Contains(t, output, "identity")
	// Should not create edges for empty strings.
	assert.NotContains(t, output, "\"\" ->")
}

func TestRenderMermaid_InvalidViaReference(t *testing.T) {
	// Test that referencing non-existent providers doesn't cause errors.
	identities := map[string]schema.Identity{
		"admin": {
			Kind: "aws/permission-set",
			Via:  &schema.IdentityVia{Provider: "nonexistent"},
		},
	}

	output, err := RenderMermaid(nil, nil, identities)
	require.NoError(t, err)

	// Should still generate valid Mermaid with edge to nonexistent provider.
	assert.Contains(t, output, "graph LR")
	assert.Contains(t, output, "admin[")
	assert.Contains(t, output, "nonexistent --> admin")
}

func TestRenderMermaid_CircularIdentityChain(t *testing.T) {
	// Test circular references (identity A -> identity B -> identity A).
	identities := map[string]schema.Identity{
		"identity-a": {
			Kind: "aws/assume-role",
			Via:  &schema.IdentityVia{Identity: "identity-b"},
		},
		"identity-b": {
			Kind: "aws/assume-role",
			Via:  &schema.IdentityVia{Identity: "identity-a"},
		},
	}

	output, err := RenderMermaid(nil, nil, identities)
	require.NoError(t, err)

	// Should generate valid Mermaid with circular edges.
	assert.Contains(t, output, "identity_a[")
	assert.Contains(t, output, "identity_b[")
	assert.Contains(t, output, "identity_b --> identity_a")
	assert.Contains(t, output, "identity_a --> identity_b")
}

func TestRenderMermaid_NilVia(t *testing.T) {
	// Test identity with nil Via.
	identities := map[string]schema.Identity{
		"standalone": {
			Kind: "aws/user",
			Via:  nil,
		},
	}

	output, err := RenderMermaid(nil, nil, identities)
	require.NoError(t, err)

	assert.Contains(t, output, "standalone[")
	assert.Contains(t, output, "aws/user")
}

func TestRenderMermaid_EmptyStringsInVia(t *testing.T) {
	// Test Via with empty string fields.
	identities := map[string]schema.Identity{
		"identity": {
			Kind: "aws/assume-role",
			Via:  &schema.IdentityVia{Provider: "", Identity: ""},
		},
	}

	output, err := RenderMermaid(nil, nil, identities)
	require.NoError(t, err)

	assert.Contains(t, output, "identity[")
	// Should not create edges for empty strings.
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		assert.NotContains(t, line, " --> identity")
	}
}

func TestRenderMermaid_MalformedProviderData(t *testing.T) {
	// Test with missing required fields.
	providers := map[string]schema.Provider{
		"provider-no-kind": {
			Kind: "", // Empty kind.
		},
	}

	output, err := RenderMermaid(nil, providers, nil)
	require.NoError(t, err)

	// Should still generate valid Mermaid.
	assert.Contains(t, output, "graph LR")
	assert.Contains(t, output, "provider_no_kind[")
}

func TestRenderMarkdown_ErrorFromMermaid(t *testing.T) {
	// RenderMarkdown calls RenderMermaid internally.
	// Test that it handles all inputs gracefully.
	providers := map[string]schema.Provider{
		"provider": {Kind: "test"},
	}
	identities := map[string]schema.Identity{
		"identity": {Kind: "test"},
	}

	output, err := RenderMarkdown(nil, providers, identities)
	require.NoError(t, err)

	// Should contain valid markdown with mermaid block.
	assert.Contains(t, output, "# Authentication Configuration")
	assert.Contains(t, output, "```mermaid")
	assert.Contains(t, output, "graph LR")
	assert.Contains(t, output, "```")
}
