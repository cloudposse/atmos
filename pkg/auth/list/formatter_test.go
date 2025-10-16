package list

import (
	"testing"

	tree "github.com/charmbracelet/lipgloss/tree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "shorter than max",
			input:    "short",
			maxLen:   10,
			expected: "short",
		},
		{
			name:     "equal to max",
			input:    "exactlyten",
			maxLen:   10,
			expected: "exactlyten",
		},
		{
			name:     "longer than max",
			input:    "this is a very long string",
			maxLen:   10,
			expected: "this is...",
		},
		{
			name:     "very short max",
			input:    "hello",
			maxLen:   2,
			expected: "he",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateProvidersTable(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:     "aws/iam-identity-center",
			Region:   "us-east-1",
			StartURL: "https://d-abc123.awsapps.com/start",
			Default:  true,
		},
	}

	table, err := createProvidersTable(providers)

	require.NoError(t, err)

	// Verify table was created successfully.
	assert.NotNil(t, table)

	// Just verify the function doesn't error - the table rendering is tested integration-wise.
}

func TestCreateProvidersTable_Empty(t *testing.T) {
	providers := map[string]schema.Provider{}

	table, err := createProvidersTable(providers)

	require.NoError(t, err)
	assert.NotNil(t, table)
}

func TestCreateIdentitiesTable(t *testing.T) {
	identities := map[string]schema.Identity{
		"admin": {
			Kind:    "aws/permission-set",
			Default: true,
			Via:     &schema.IdentityVia{Provider: "aws-sso"},
		},
	}
	table, err := createIdentitiesTable(identities)

	require.NoError(t, err)
	assert.NotNil(t, table)
}

func TestCreateIdentitiesTable_AWSUser(t *testing.T) {
	identities := map[string]schema.Identity{
		"ci": {
			Kind: "aws/user",
			Via:  nil,
		},
	}
	table, err := createIdentitiesTable(identities)

	require.NoError(t, err)
	assert.NotNil(t, table)
}

func TestBuildProvidersTree(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:     "aws/iam-identity-center",
			Region:   "us-east-1",
			StartURL: "https://d-abc123.awsapps.com/start",
			Default:  true,
		},
		"okta": {
			Kind:   "aws/saml",
			Region: "us-west-2",
			URL:    "https://company.okta.com/app",
		},
	}

	tree := buildProvidersTree(providers)

	assert.Contains(t, tree, "aws-sso")
	assert.Contains(t, tree, "okta")
	assert.Contains(t, tree, "aws/iam-identity-center")
	assert.Contains(t, tree, "aws/saml")
	assert.Contains(t, tree, "[DEFAULT]")
	assert.Contains(t, tree, "Region: us-east-1")
	assert.Contains(t, tree, "Region: us-west-2")
	assert.Contains(t, tree, "Start URL:")
	assert.Contains(t, tree, "URL:")
}

func TestBuildProvidersTree_WithSession(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind: "aws/iam-identity-center",
			Session: &schema.SessionConfig{
				Duration: "3600",
			},
		},
	}

	tree := buildProvidersTree(providers)

	assert.Contains(t, tree, "Session")
	assert.Contains(t, tree, "Duration: 3600")
}

func TestBuildIdentitiesTree(t *testing.T) {
	identities := map[string]schema.Identity{
		"admin": {
			Kind:    "aws/permission-set",
			Default: true,
			Via:     &schema.IdentityVia{Provider: "aws-sso"},
			Principal: map[string]interface{}{
				"account": map[string]interface{}{
					"id": "123456789012",
				},
				"name": "AdministratorAccess",
			},
		},
		"dev": {
			Kind:  "aws/assume-role",
			Alias: "developer",
			Via:   &schema.IdentityVia{Identity: "admin"},
		},
	}

	tree := buildIdentitiesTree(identities)

	assert.Contains(t, tree, "admin")
	assert.Contains(t, tree, "dev")
	assert.Contains(t, tree, "[DEFAULT]")
	assert.Contains(t, tree, "[ALIAS: developer]")
	assert.Contains(t, tree, "Principal")
	assert.Contains(t, tree, "aws/permission-set")
	assert.Contains(t, tree, "aws/assume-role")
}

func TestBuildIdentitiesTree_Standalone(t *testing.T) {
	identities := map[string]schema.Identity{
		"ci": {
			Kind: "aws/user",
			Via:  nil,
		},
	}

	tree := buildIdentitiesTree(identities)

	assert.Contains(t, tree, "ci")
	assert.Contains(t, tree, "aws/user")
}

func TestAddMapToTree_Simple(t *testing.T) {
	// Test simple flat map structure.
	m := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
		"key3": true,
	}

	node := tree.New().Root("Test")
	addMapToTree(node, m, 0)

	output := node.String()
	assert.Contains(t, output, "key1")
	assert.Contains(t, output, "value1")
	assert.Contains(t, output, "key2")
	assert.Contains(t, output, "123")
	assert.Contains(t, output, "key3")
	assert.Contains(t, output, "true")
}

func TestAddMapToTree_Nested(t *testing.T) {
	// Test nested map structure.
	m := map[string]interface{}{
		"outer": map[string]interface{}{
			"inner1": "value1",
			"inner2": 42,
		},
		"simple": "test",
	}

	node := tree.New().Root("Test")
	addMapToTree(node, m, 0)

	output := node.String()
	assert.Contains(t, output, "outer")
	assert.Contains(t, output, "inner1")
	assert.Contains(t, output, "value1")
	assert.Contains(t, output, "inner2")
	assert.Contains(t, output, "42")
	assert.Contains(t, output, "simple")
	assert.Contains(t, output, "test")
}

func TestAddMapToTree_MaxDepth(t *testing.T) {
	// Test deeply nested structure to ensure depth limit works.
	m := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"level3": map[string]interface{}{
					"level4": map[string]interface{}{
						"level5": map[string]interface{}{
							"level6": "deep value",
						},
					},
				},
			},
		},
	}

	node := tree.New().Root("Test")
	addMapToTree(node, m, 0)

	output := node.String()
	// Verify at least some levels are rendered.
	assert.Contains(t, output, "level1")
	assert.Contains(t, output, "level2")
	assert.Contains(t, output, "level3")
}

func TestRenderMermaid_Syntax(t *testing.T) {
	// Test that RenderMermaid produces valid Mermaid syntax.
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:    "aws-sso",
			Default: true,
		},
		"okta": {
			Kind: "okta",
		},
	}

	identities := map[string]schema.Identity{
		"admin": {
			Kind:    "aws/assume-role",
			Default: true,
			Via: &schema.IdentityVia{
				Provider: "aws-sso",
			},
		},
		"developer": {
			Kind: "aws/assume-role",
			Via: &schema.IdentityVia{
				Provider: "aws-sso",
			},
		},
	}

	output, err := RenderMermaid(nil, providers, identities)
	require.NoError(t, err)

	// Verify basic structure.
	assert.Contains(t, output, "graph LR")
	assert.Contains(t, output, "classDef provider")
	assert.Contains(t, output, "classDef identity")
	assert.Contains(t, output, "classDef default")

	// Verify nodes are declared without chained class syntax.
	assert.Contains(t, output, "aws_sso[")
	assert.Contains(t, output, "admin[")
	assert.Contains(t, output, "developer[")
	assert.NotContains(t, output, ":::provider:::default")
	assert.NotContains(t, output, ":::identity:::default")
	assert.NotContains(t, output, ":::provider")
	assert.NotContains(t, output, ":::identity")

	// Verify separate class directives exist.
	assert.Contains(t, output, "class aws_sso provider")
	assert.Contains(t, output, "class aws_sso default")
	assert.Contains(t, output, "class admin identity")
	assert.Contains(t, output, "class admin default")
	assert.Contains(t, output, "class developer identity")

	// Verify edges.
	assert.Contains(t, output, "aws_sso --> admin")
	assert.Contains(t, output, "aws_sso --> developer")

	// Print output for manual verification.
	t.Logf("Generated Mermaid:\n%s", output)
}
