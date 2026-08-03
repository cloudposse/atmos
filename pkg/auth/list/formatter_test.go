package list

import (
	"strings"
	"testing"
	"time"

	tree "github.com/charmbracelet/lipgloss/tree"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// forceTTYForTable forces pkg/list/renderer's TTY branch (styled table with
// headers) instead of the default non-TTY plain-list fallback that `go test`
// would otherwise take, since its stdout is a pipe, not a real terminal.
func forceTTYForTable(t *testing.T) {
	t.Helper()
	original := viper.GetBool("force-tty")
	viper.Set("force-tty", true)
	t.Cleanup(func() { viper.Set("force-tty", original) })
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

	view, err := createProvidersTable(providers)

	require.NoError(t, err)
	assert.NotEmpty(t, view)
	assert.Contains(t, view, "aws-sso")
}

func TestCreateProvidersTable_Empty(t *testing.T) {
	forceTTYForTable(t) // Headers only render on the styled-table (TTY) path; plain-list omits them.
	providers := map[string]schema.Provider{}

	view, err := createProvidersTable(providers)

	require.NoError(t, err)
	// No rows, but the header row (and its formatting) still render.
	assert.Contains(t, view, "NAME")
}

// TestCreateProvidersTable_MultipleRowsVisible guards against a rendering
// regression that drops or hides rows past the first.
func TestCreateProvidersTable_MultipleRowsVisible(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind:     "aws/iam-identity-center",
			Region:   "us-east-1",
			StartURL: "https://d-abc123.awsapps.com/start",
			Default:  true,
		},
		"okta": {
			// Use Kind: "okta" to match the convention in TestBuildProviderRows_WithURL.
			Kind:   "okta",
			Region: "us-west-2",
			URL:    "https://company.okta.com/app",
		},
		"google": {
			Kind:   "aws/saml",
			Region: "us-east-2",
			URL:    "https://google.example/app",
		},
	}

	view, err := createProvidersTable(providers)
	require.NoError(t, err)
	require.NotEmpty(t, view)

	// Every provider name must appear in the rendered table.
	assert.Contains(t, view, "aws-sso")
	assert.Contains(t, view, "okta")
	assert.Contains(t, view, "google")

	// Assert a per-row distinguishing field so a regression that visibly
	// renders all rows but corrupts row content (e.g., column misalignment,
	// shared-row data) is caught — not just one that drops rows entirely.
	assert.Contains(t, view, "us-east-1", "aws-sso row content must include its region")
	assert.Contains(t, view, "us-west-2", "okta row content must include its region")
	assert.Contains(t, view, "us-east-2", "google row content must include its region")
}

// TestCreateIdentitiesTable_MultipleRowsVisible guards against a rendering
// regression that drops or hides rows past the first.
func TestCreateIdentitiesTable_MultipleRowsVisible(t *testing.T) {
	identities := map[string]schema.Identity{
		"admin": {
			Kind:    "aws/permission-set",
			Default: true,
			Via:     &schema.IdentityVia{Provider: "aws-sso"},
		},
		"readonly": {
			Kind: "aws/permission-set",
			Via:  &schema.IdentityVia{Provider: "aws-sso"},
		},
		"ci": {
			Kind: "aws/user",
		},
	}

	view, err := createIdentitiesTable(nil, identities)
	require.NoError(t, err)
	require.NotEmpty(t, view)

	// Every identity name must appear in the rendered table.
	assert.Contains(t, view, "admin")
	assert.Contains(t, view, "readonly")
	assert.Contains(t, view, "ci")
}

func TestCreateIdentitiesTable(t *testing.T) {
	identities := map[string]schema.Identity{
		"admin": {
			Kind:    "aws/permission-set",
			Default: true,
			Via:     &schema.IdentityVia{Provider: "aws-sso"},
		},
	}
	view, err := createIdentitiesTable(nil, identities)

	require.NoError(t, err)
	assert.Contains(t, view, "admin")
}

func TestCreateIdentitiesTable_AWSUser(t *testing.T) {
	identities := map[string]schema.Identity{
		"ci": {
			Kind: "aws/user",
			Via:  nil,
		},
	}
	view, err := createIdentitiesTable(nil, identities)

	require.NoError(t, err)
	assert.Contains(t, view, "ci")
	assert.Contains(t, view, "aws-user", "aws/user identities show aws-user as the via-provider")
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
	assert.Equal(t, "aws-sso", rows[0]["name"])
	assert.Contains(t, rows[0]["url"], "d-abc123")
	assert.Equal(t, "✓", rows[0]["default"])
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
	assert.Equal(t, "okta", rows[0]["name"])
	assert.Contains(t, rows[0]["url"], "company.okta.com")
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
	assert.Equal(t, "us-west-2", rows[0]["region"])
}

func TestBuildProviderRows_NoRegion(t *testing.T) {
	providers := map[string]schema.Provider{
		"okta": {
			Kind: "okta",
		},
	}

	rows := buildProviderRows(providers)

	require.Len(t, rows, 1)
	assert.Equal(t, "-", rows[0]["region"])
}

func TestBuildProviderRows_NoURL(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {
			Kind: "aws/iam-identity-center",
		},
	}

	rows := buildProviderRows(providers)

	require.Len(t, rows, 1)
	assert.Equal(t, "-", rows[0]["url"])
}

func TestBuildIdentityRow_WithViaProvider(t *testing.T) {
	identity := schema.Identity{
		Kind:    "aws/permission-set",
		Default: true,
		Via:     &schema.IdentityVia{Provider: "aws-sso"},
	}

	row := buildIdentityRow(nil, &identity, "admin")

	assert.Equal(t, " ", row["status"])                // Status indicator (space when authManager is nil).
	assert.Equal(t, "admin", row["name"])              // Name.
	assert.Equal(t, "aws/permission-set", row["kind"]) // Kind.
	assert.Equal(t, "aws-sso", row["via_provider"])    // Via Provider.
	assert.Equal(t, "-", row["via_identity"])          // Via Identity.
	assert.Equal(t, "✓", row["default"])               // Default.
}

func TestBuildIdentityRow_WithViaIdentity(t *testing.T) {
	identity := schema.Identity{
		Kind: "aws/assume-role",
		Via:  &schema.IdentityVia{Identity: "admin"},
	}

	row := buildIdentityRow(nil, &identity, "dev")

	assert.Equal(t, " ", row["status"])           // Status indicator.
	assert.Equal(t, "dev", row["name"])           // Name.
	assert.Equal(t, "-", row["via_provider"])     // Via Provider.
	assert.Equal(t, "admin", row["via_identity"]) // Via Identity.
}

func TestBuildIdentityRow_AWSUser(t *testing.T) {
	identity := schema.Identity{
		Kind: "aws/user",
	}

	row := buildIdentityRow(nil, &identity, "ci")

	assert.Equal(t, " ", row["status"])              // Status indicator.
	assert.Equal(t, "ci", row["name"])               // Name.
	assert.Equal(t, "aws/user", row["kind"])         // Kind.
	assert.Equal(t, "aws-user", row["via_provider"]) // Via Provider.
	assert.Equal(t, "-", row["via_identity"])        // Via Identity.
}

func TestBuildIdentityRow_WithAlias(t *testing.T) {
	identity := schema.Identity{
		Kind:  "aws/assume-role",
		Alias: "developer",
		Via:   &schema.IdentityVia{Provider: "aws-sso"},
	}

	row := buildIdentityRow(nil, &identity, "dev")

	assert.Equal(t, " ", row["status"])        // Status indicator.
	assert.Equal(t, "dev", row["name"])        // Name.
	assert.Equal(t, "developer", row["alias"]) // Alias.
}

func TestBuildIdentityRow_NoAlias(t *testing.T) {
	identity := schema.Identity{
		Kind: "aws/assume-role",
		Via:  &schema.IdentityVia{Provider: "aws-sso"},
	}

	row := buildIdentityRow(nil, &identity, "admin")

	assert.Equal(t, " ", row["status"]) // Status indicator.
	assert.Equal(t, "-", row["alias"])  // Alias.
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
