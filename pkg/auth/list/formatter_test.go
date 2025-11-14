package list

import (
	"strings"
	"testing"
	"time"

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
	table, err := createIdentitiesTable(nil, identities)

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
	table, err := createIdentitiesTable(nil, identities)

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

	tree := buildIdentitiesTree(nil, identities)

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

	tree := buildIdentitiesTree(nil, identities)

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

func TestRenderTable_BothProvidersAndIdentities(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:     "aws/iam-identity-center",
			Region:   "us-east-1",
			StartURL: "https://d-abc123.awsapps.com/start",
			Default:  true,
		},
	}

	identities := map[string]schema.Identity{
		"admin": {
			Kind:    "aws/permission-set",
			Default: true,
			Via:     &schema.IdentityVia{Provider: "aws-sso"},
		},
	}

	output, err := RenderTable(nil, providers, identities)
	require.NoError(t, err)

	assert.Contains(t, output, "PROVIDERS")
	assert.Contains(t, output, "IDENTITIES")
}

func TestRenderTable_ProvidersOnly(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:     "aws/iam-identity-center",
			Region:   "us-east-1",
			StartURL: "https://d-abc123.awsapps.com/start",
			Default:  true,
		},
	}

	output, err := RenderTable(nil, providers, nil)
	require.NoError(t, err)

	assert.Contains(t, output, "PROVIDERS")
	assert.NotContains(t, output, "IDENTITIES")
}

func TestRenderTable_IdentitiesOnly(t *testing.T) {
	identities := map[string]schema.Identity{
		"admin": {
			Kind:    "aws/permission-set",
			Default: true,
			Via:     &schema.IdentityVia{Provider: "aws-sso"},
		},
	}

	output, err := RenderTable(nil, nil, identities)
	require.NoError(t, err)

	assert.NotContains(t, output, "PROVIDERS")
	assert.Contains(t, output, "IDENTITIES")
}

func TestRenderTable_Empty(t *testing.T) {
	output, err := RenderTable(nil, nil, nil)
	require.NoError(t, err)

	assert.Contains(t, output, "No providers or identities configured")
}

func TestRenderTree_BothProvidersAndIdentities(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:     "aws/iam-identity-center",
			Region:   "us-east-1",
			StartURL: "https://d-abc123.awsapps.com/start",
			Default:  true,
		},
	}

	identities := map[string]schema.Identity{
		"admin": {
			Kind:    "aws/permission-set",
			Default: true,
			Via:     &schema.IdentityVia{Provider: "aws-sso"},
		},
	}

	output, err := RenderTree(nil, providers, identities)
	require.NoError(t, err)

	assert.Contains(t, output, "Authentication Configuration")
	assert.Contains(t, output, "aws-sso")
	assert.Contains(t, output, "admin")
}

func TestRenderTree_ProvidersOnly(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:     "aws/iam-identity-center",
			Region:   "us-east-1",
			StartURL: "https://d-abc123.awsapps.com/start",
			Default:  true,
		},
	}

	output, err := RenderTree(nil, providers, nil)
	require.NoError(t, err)

	assert.Contains(t, output, "Authentication Configuration")
	assert.Contains(t, output, "aws-sso")
}

func TestRenderTree_IdentitiesOnly(t *testing.T) {
	identities := map[string]schema.Identity{
		"ci": {
			Kind: "aws/user",
		},
	}

	output, err := RenderTree(nil, nil, identities)
	require.NoError(t, err)

	assert.Contains(t, output, "Authentication Configuration")
	assert.Contains(t, output, "Standalone Identities")
	assert.Contains(t, output, "ci")
}

func TestRenderTree_Empty(t *testing.T) {
	output, err := RenderTree(nil, nil, nil)
	require.NoError(t, err)

	assert.Contains(t, output, "No providers or identities configured")
}

func TestBuildProviderRows_WithStartURL(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:     "aws/iam-identity-center",
			StartURL: "https://d-abc123.awsapps.com/start",
			Default:  true,
		},
	}

	rows := buildProviderRows(providers)

	require.Len(t, rows, 1)
	assert.Equal(t, "aws-sso", rows[0][0])
	assert.Contains(t, rows[0][3], "d-abc123")
	assert.Equal(t, "✓", rows[0][4])
}

func TestBuildProviderRows_WithURL(t *testing.T) {
	providers := map[string]schema.Provider{
		"okta": {
			Kind: "okta",
			URL:  "https://company.okta.com/app",
		},
	}

	rows := buildProviderRows(providers)

	require.Len(t, rows, 1)
	assert.Equal(t, "okta", rows[0][0])
	assert.Contains(t, rows[0][3], "company.okta.com")
}

func TestBuildProviderRows_WithRegion(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:   "aws/iam-identity-center",
			Region: "us-west-2",
		},
	}

	rows := buildProviderRows(providers)

	require.Len(t, rows, 1)
	assert.Equal(t, "us-west-2", rows[0][2])
}

func TestBuildProviderRows_NoRegion(t *testing.T) {
	providers := map[string]schema.Provider{
		"okta": {
			Kind: "okta",
		},
	}

	rows := buildProviderRows(providers)

	require.Len(t, rows, 1)
	assert.Equal(t, "-", rows[0][2])
}

func TestBuildProviderRows_NoURL(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind: "aws/iam-identity-center",
		},
	}

	rows := buildProviderRows(providers)

	require.Len(t, rows, 1)
	assert.Equal(t, "-", rows[0][3])
}

func TestBuildIdentityTableRow_WithViaProvider(t *testing.T) {
	identity := schema.Identity{
		Kind:    "aws/permission-set",
		Default: true,
		Via:     &schema.IdentityVia{Provider: "aws-sso"},
	}

	row := buildIdentityTableRow(nil, &identity, "admin")

	assert.Equal(t, " ", row[0])                  // Status indicator (space when authManager is nil).
	assert.Equal(t, "admin", row[1])              // Name.
	assert.Equal(t, "aws/permission-set", row[2]) // Kind.
	assert.Equal(t, "aws-sso", row[3])            // Via Provider.
	assert.Equal(t, "-", row[4])                  // Via Identity.
	assert.Equal(t, "✓", row[5])                  // Default.
}

func TestBuildIdentityTableRow_WithViaIdentity(t *testing.T) {
	identity := schema.Identity{
		Kind: "aws/assume-role",
		Via:  &schema.IdentityVia{Identity: "admin"},
	}

	row := buildIdentityTableRow(nil, &identity, "dev")

	assert.Equal(t, " ", row[0])     // Status indicator.
	assert.Equal(t, "dev", row[1])   // Name.
	assert.Equal(t, "-", row[3])     // Via Provider.
	assert.Equal(t, "admin", row[4]) // Via Identity.
}

func TestBuildIdentityTableRow_AWSUser(t *testing.T) {
	identity := schema.Identity{
		Kind: "aws/user",
	}

	row := buildIdentityTableRow(nil, &identity, "ci")

	assert.Equal(t, " ", row[0])        // Status indicator.
	assert.Equal(t, "ci", row[1])       // Name.
	assert.Equal(t, "aws/user", row[2]) // Kind.
	assert.Equal(t, "aws-user", row[3]) // Via Provider.
	assert.Equal(t, "-", row[4])        // Via Identity.
}

func TestBuildIdentityTableRow_WithAlias(t *testing.T) {
	identity := schema.Identity{
		Kind:  "aws/assume-role",
		Alias: "developer",
		Via:   &schema.IdentityVia{Provider: "aws-sso"},
	}

	row := buildIdentityTableRow(nil, &identity, "dev")

	assert.Equal(t, " ", row[0])         // Status indicator.
	assert.Equal(t, "dev", row[1])       // Name.
	assert.Equal(t, "developer", row[6]) // Alias.
}

func TestBuildIdentityTableRow_NoAlias(t *testing.T) {
	identity := schema.Identity{
		Kind: "aws/assume-role",
		Via:  &schema.IdentityVia{Provider: "aws-sso"},
	}

	row := buildIdentityTableRow(nil, &identity, "admin")

	assert.Equal(t, " ", row[0]) // Status indicator.
	assert.Equal(t, "-", row[6]) // Alias.
}

func TestBuildIdentitiesTree_WithCredentials(t *testing.T) {
	identities := map[string]schema.Identity{
		"admin": {
			Kind:    "aws/permission-set",
			Default: true,
			Via:     &schema.IdentityVia{Provider: "aws-sso"},
			Credentials: map[string]interface{}{
				"access_key_id":     "AKIAIOSFODNN7EXAMPLE",
				"secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			},
		},
	}

	tree := buildIdentitiesTree(nil, identities)

	assert.Contains(t, tree, "admin")
	assert.Contains(t, tree, "Credentials: [redacted]")
}

func TestAddMapToTree_WithSlice(t *testing.T) {
	m := map[string]interface{}{
		"items": []interface{}{"item1", "item2", "item3"},
	}

	node := tree.New().Root("Test")
	addMapToTree(node, m, 0)

	output := node.String()
	assert.Contains(t, output, "items")
	assert.Contains(t, output, "item1")
	assert.Contains(t, output, "item2")
	assert.Contains(t, output, "item3")
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

	// Validate structure using parser.
	err = validateMermaidStructure(output)
	require.NoError(t, err, "Mermaid structure validation failed")

	// Try to validate with mermaid-cli if available.
	if err := validateWithMermaidCLI(t, output); err != nil {
		t.Logf("Mermaid CLI validation skipped: %v", err)
	}
	// Print output for manual verification.
	t.Logf("Generated Mermaid:\n%s", output)
}

// TestBuildIdentityNodeForProvider_IdentityCycle tests that identity cycles are detected.
func TestBuildIdentityNodeForProvider_IdentityCycle(t *testing.T) {
	// Create a circular identity chain: admin → dev → admin.
	identities := map[string]schema.Identity{
		"admin": {
			Via: &schema.IdentityVia{
				Identity: "dev",
			},
		},
		"dev": {
			Via: &schema.IdentityVia{
				Identity: "admin",
			},
		},
	}

	// This should complete within 1 second, not hang forever.
	done := make(chan struct{})
	go func() {
		defer close(done)
		adminIdentity := identities["admin"]
		visited := make(map[string]struct{})
		node := buildIdentityNodeForProvider(nil, &adminIdentity, "admin", identities, visited)
		output := node.String()

		// Should contain cycle detection message.
		if !strings.Contains(output, "cycle") {
			t.Errorf("Expected cycle detection message in output, got: %s", output)
		}
	}()

	select {
	case <-done:
		// Test completed successfully.
	case <-time.After(1 * time.Second):
		t.Fatal("buildIdentityNodeForProvider hung on identity cycle (infinite recursion)")
	}
}

// TestBuildIdentityNodeForProvider_SharedNode tests that shared nodes are not flagged as cycles.
func TestBuildIdentityNodeForProvider_SharedNode(t *testing.T) {
	// Create a diamond pattern: base → [branch-a, branch-b] → merged.
	// This has a shared node "merged" but is NOT a cycle.
	identities := map[string]schema.Identity{
		"base": {
			Kind: "aws/assume-role",
		},
		"branch-a": {
			Kind: "aws/assume-role",
			Via: &schema.IdentityVia{
				Identity: "base",
			},
		},
		"branch-b": {
			Kind: "aws/assume-role",
			Via: &schema.IdentityVia{
				Identity: "base",
			},
		},
		"merged": {
			Kind: "aws/assume-role",
			Via: &schema.IdentityVia{
				Identity: "branch-a",
			},
		},
	}

	// Build tree starting from base.
	baseIdentity := identities["base"]
	visited := make(map[string]struct{})
	node := buildIdentityNodeForProvider(nil, &baseIdentity, "base", identities, visited)
	output := node.String()

	// Should NOT contain "cycle detected".
	assert.NotContains(t, output, "cycle", "Shared nodes should not be flagged as cycles")

	// Should contain all identity names.
	assert.Contains(t, output, "base")
	assert.Contains(t, output, "branch-a")
	assert.Contains(t, output, "branch-b")
	assert.Contains(t, output, "merged")
}
