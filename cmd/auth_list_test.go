package cmd

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestParseFilterFlags_NoFlags(t *testing.T) {
	opts := &flags.StandardOptions{}

	filters, err := parseFilterFlags(opts)

	require.NoError(t, err)
	assert.False(t, filters.showProvidersOnly)
	assert.False(t, filters.showIdentitiesOnly)
	assert.Empty(t, filters.providerNames)
	assert.Empty(t, filters.identityNames)
}

// TestParseFilterFlags_ProvidersOnly removed - with StandardOptions, empty string means "not set".
// To show all providers, user must explicitly pass provider names or use a special value.
// The current behavior is: no flag = show both, --providers=name1,name2 = show those providers only.

func TestParseFilterFlags_ProvidersWithNames(t *testing.T) {
	opts := &flags.StandardOptions{
		Providers: "aws-sso,okta",
	}

	filters, err := parseFilterFlags(opts)

	require.NoError(t, err)
	assert.True(t, filters.showProvidersOnly)
	assert.Equal(t, []string{"aws-sso", "okta"}, filters.providerNames)
}

func TestParseFilterFlags_ProvidersWithSpaces(t *testing.T) {
	opts := &flags.StandardOptions{
		Providers: " aws-sso , okta ",
	}

	filters, err := parseFilterFlags(opts)

	require.NoError(t, err)
	assert.True(t, filters.showProvidersOnly)
	assert.Equal(t, []string{"aws-sso", "okta"}, filters.providerNames)
}

// TestParseFilterFlags_IdentitiesOnly removed - with StandardOptions, empty string means "not set".
// To show all identities, user must explicitly pass identity names or use a special value.
// The current behavior is: no flag = show both, --identities=name1,name2 = show those identities only.

func TestParseFilterFlags_IdentitiesWithNames(t *testing.T) {
	opts := &flags.StandardOptions{
		Identities: "admin,dev,prod",
	}

	filters, err := parseFilterFlags(opts)

	require.NoError(t, err)
	assert.True(t, filters.showIdentitiesOnly)
	assert.Equal(t, []string{"admin", "dev", "prod"}, filters.identityNames)
}

func TestParseFilterFlags_MutuallyExclusive(t *testing.T) {
	opts := &flags.StandardOptions{
		Providers:  "aws-sso",
		Identities: "admin",
	}

	_, err := parseFilterFlags(opts)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

// createTestAuthListCmd creates a fresh command instance for testing.
func createTestAuthListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "list",
	}
	cmd.Flags().String("providers", "", "")
	cmd.Flags().String("identities", "", "")
	cmd.Flags().StringP("format", "f", "table", "")
	return cmd
}

func TestApplyFilters_NoFilters(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
		"okta":    {Kind: "aws/saml"},
	}
	identities := map[string]schema.Identity{
		"admin": {Kind: "aws/permission-set"},
		"dev":   {Kind: "aws/permission-set"},
	}
	filters := &filterConfig{}

	filteredProviders, filteredIdentities, err := applyFilters(providers, identities, filters)

	require.NoError(t, err)
	assert.Equal(t, providers, filteredProviders)
	assert.Equal(t, identities, filteredIdentities)
}

func TestApplyFilters_ProvidersOnly(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
		"okta":    {Kind: "aws/saml"},
	}
	identities := map[string]schema.Identity{
		"admin": {Kind: "aws/permission-set"},
	}
	filters := &filterConfig{
		showProvidersOnly: true,
	}

	filteredProviders, filteredIdentities, err := applyFilters(providers, identities, filters)

	require.NoError(t, err)
	assert.Equal(t, providers, filteredProviders)
	assert.Empty(t, filteredIdentities)
}

func TestApplyFilters_SpecificProvider(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
		"okta":    {Kind: "aws/saml"},
	}
	identities := map[string]schema.Identity{
		"admin": {Kind: "aws/permission-set"},
	}
	filters := &filterConfig{
		showProvidersOnly: true,
		providerNames:     []string{"aws-sso"},
	}

	filteredProviders, filteredIdentities, err := applyFilters(providers, identities, filters)

	require.NoError(t, err)
	assert.Len(t, filteredProviders, 1)
	assert.Contains(t, filteredProviders, "aws-sso")
	assert.Empty(t, filteredIdentities)
}

func TestApplyFilters_NonExistentProvider(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
	}
	identities := map[string]schema.Identity{}
	filters := &filterConfig{
		showProvidersOnly: true,
		providerNames:     []string{"nonexistent"},
	}

	_, _, err := applyFilters(providers, identities, filters)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestApplyFilters_IdentitiesOnly(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
	}
	identities := map[string]schema.Identity{
		"admin": {Kind: "aws/permission-set"},
		"dev":   {Kind: "aws/permission-set"},
	}
	filters := &filterConfig{
		showIdentitiesOnly: true,
	}

	filteredProviders, filteredIdentities, err := applyFilters(providers, identities, filters)

	require.NoError(t, err)
	assert.Empty(t, filteredProviders)
	assert.Equal(t, identities, filteredIdentities)
}

func TestApplyFilters_SpecificIdentity(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
	}
	identities := map[string]schema.Identity{
		"admin": {Kind: "aws/permission-set"},
		"dev":   {Kind: "aws/permission-set"},
	}
	filters := &filterConfig{
		showIdentitiesOnly: true,
		identityNames:      []string{"admin"},
	}

	filteredProviders, filteredIdentities, err := applyFilters(providers, identities, filters)

	require.NoError(t, err)
	assert.Empty(t, filteredProviders)
	assert.Len(t, filteredIdentities, 1)
	assert.Contains(t, filteredIdentities, "admin")
}

func TestApplyFilters_NonExistentIdentity(t *testing.T) {
	providers := map[string]schema.Provider{}
	identities := map[string]schema.Identity{
		"admin": {Kind: "aws/permission-set"},
	}
	filters := &filterConfig{
		showIdentitiesOnly: true,
		identityNames:      []string{"nonexistent"},
	}

	_, _, err := applyFilters(providers, identities, filters)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRenderJSON_Empty(t *testing.T) {
	providers := map[string]schema.Provider{}
	identities := map[string]schema.Identity{}

	output, err := renderJSON(providers, identities)

	require.NoError(t, err)
	assert.Equal(t, "{}\n", output)
}

func TestRenderJSON_ProvidersOnly(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center", Region: "us-east-1"},
	}
	identities := map[string]schema.Identity{}

	output, err := renderJSON(providers, identities)

	require.NoError(t, err)
	assert.Contains(t, output, `"providers"`)
	assert.Contains(t, output, `"aws-sso"`)
	assert.NotContains(t, output, `"identities"`)
}

func TestRenderJSON_IdentitiesOnly(t *testing.T) {
	providers := map[string]schema.Provider{}
	identities := map[string]schema.Identity{
		"admin": {Kind: "aws/permission-set", Default: true},
	}

	output, err := renderJSON(providers, identities)

	require.NoError(t, err)
	assert.Contains(t, output, `"identities"`)
	assert.Contains(t, output, `"admin"`)
	assert.NotContains(t, output, `"providers"`)
}

func TestRenderJSON_Both(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
	}
	identities := map[string]schema.Identity{
		"admin": {Kind: "aws/permission-set"},
	}

	output, err := renderJSON(providers, identities)

	require.NoError(t, err)
	assert.Contains(t, output, `"providers"`)
	assert.Contains(t, output, `"identities"`)
}

func TestRenderYAML_Empty(t *testing.T) {
	providers := map[string]schema.Provider{}
	identities := map[string]schema.Identity{}

	output, err := renderYAML(providers, identities)

	require.NoError(t, err)
	assert.Equal(t, "{}\n", output)
}

func TestRenderYAML_ProvidersOnly(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center", Region: "us-east-1"},
	}
	identities := map[string]schema.Identity{}

	output, err := renderYAML(providers, identities)

	require.NoError(t, err)
	assert.Contains(t, output, "providers:")
	assert.Contains(t, output, "aws-sso:")
	assert.NotContains(t, output, "identities:")
}

func TestRenderYAML_IdentitiesOnly(t *testing.T) {
	providers := map[string]schema.Provider{}
	identities := map[string]schema.Identity{
		"admin": {Kind: "aws/permission-set", Default: true},
	}

	output, err := renderYAML(providers, identities)

	require.NoError(t, err)
	assert.Contains(t, output, "identities:")
	assert.Contains(t, output, "admin:")
	assert.NotContains(t, output, "providers:")
}

func TestRenderYAML_Both(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
	}
	identities := map[string]schema.Identity{
		"admin": {Kind: "aws/permission-set"},
	}

	output, err := renderYAML(providers, identities)

	require.NoError(t, err)
	assert.Contains(t, output, "providers:")
	assert.Contains(t, output, "identities:")
}

func TestApplyFilters_MultipleProviders(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
		"okta":    {Kind: "aws/saml"},
		"azure":   {Kind: "azure-ad"},
	}
	identities := map[string]schema.Identity{
		"admin": {Kind: "aws/permission-set"},
	}
	filters := &filterConfig{
		showProvidersOnly: true,
		providerNames:     []string{"aws-sso", "okta"},
	}

	filteredProviders, filteredIdentities, err := applyFilters(providers, identities, filters)

	require.NoError(t, err)
	assert.Len(t, filteredProviders, 2)
	assert.Contains(t, filteredProviders, "aws-sso")
	assert.Contains(t, filteredProviders, "okta")
	assert.NotContains(t, filteredProviders, "azure")
	assert.Empty(t, filteredIdentities)
}

func TestApplyFilters_MultipleIdentities(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
	}
	identities := map[string]schema.Identity{
		"admin": {Kind: "aws/permission-set"},
		"dev":   {Kind: "aws/permission-set"},
		"prod":  {Kind: "aws/permission-set"},
	}
	filters := &filterConfig{
		showIdentitiesOnly: true,
		identityNames:      []string{"admin", "prod"},
	}

	filteredProviders, filteredIdentities, err := applyFilters(providers, identities, filters)

	require.NoError(t, err)
	assert.Empty(t, filteredProviders)
	assert.Len(t, filteredIdentities, 2)
	assert.Contains(t, filteredIdentities, "admin")
	assert.Contains(t, filteredIdentities, "prod")
	assert.NotContains(t, filteredIdentities, "dev")
}

func TestProvidersFlagCompletion_ReturnsSortedProviders(t *testing.T) {
	// Setup: Create a test config file with providers in non-alphabetical order.
	tempDir := t.TempDir()
	configFile := tempDir + "/atmos.yaml"

	configContent := `auth:
  providers:
    zebra-provider:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://example.awsapps.com/start
    apple-provider:
      kind: aws/saml
      region: us-east-1
      url: https://idp.example.com/saml
    mango-provider:
      kind: azure/oidc
      audience: https://example.com
`
	err := os.WriteFile(configFile, []byte(configContent), 0o600)
	require.NoError(t, err)

	// Set environment to use test config (automatically reverted after test).
	t.Chdir(tempDir)

	cmd := createTestAuthListCmd()

	// Execute completion function.
	results, directive := providersFlagCompletion(cmd, []string{}, "")

	// Verify results are sorted alphabetically.
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Equal(t, []string{"apple-provider", "mango-provider", "zebra-provider"}, results)
}

func TestIdentitiesFlagCompletion_ReturnsSortedIdentities(t *testing.T) {
	// Setup: Create a test config file with identities in non-alphabetical order.
	tempDir := t.TempDir()
	configFile := tempDir + "/atmos.yaml"

	configContent := `auth:
  providers:
    test-provider:
      kind: aws/iam-identity-center
  identities:
    zebra-identity:
      kind: aws/permission-set
      via:
        provider: test-provider
    apple-identity:
      kind: aws/permission-set
      via:
        provider: test-provider
    mango-identity:
      kind: aws/permission-set
      via:
        provider: test-provider
`
	err := os.WriteFile(configFile, []byte(configContent), 0o600)
	require.NoError(t, err)

	// Set environment to use test config (automatically reverted after test).
	t.Chdir(tempDir)

	cmd := createTestAuthListCmd()

	// Execute completion function.
	results, directive := identitiesFlagCompletion(cmd, []string{}, "")

	// Verify results are sorted alphabetically.
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	assert.Equal(t, []string{"apple-identity", "mango-identity", "zebra-identity"}, results)
}
