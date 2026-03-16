package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestParseFilterFlags_NoFlags(t *testing.T) {
	cmd := createTestAuthListCmd()

	filters, err := parseFilterFlags(cmd)

	require.NoError(t, err)
	assert.False(t, filters.showProvidersOnly)
	assert.False(t, filters.showIdentitiesOnly)
	assert.Empty(t, filters.providerNames)
	assert.Empty(t, filters.identityNames)
}

func TestParseFilterFlags_ProvidersOnly(t *testing.T) {
	cmd := createTestAuthListCmd()
	cmd.Flags().Set("providers", "")
	cmd.Flags().Lookup("providers").Changed = true

	filters, err := parseFilterFlags(cmd)

	require.NoError(t, err)
	assert.True(t, filters.showProvidersOnly)
	assert.False(t, filters.showIdentitiesOnly)
	assert.Empty(t, filters.providerNames)
	assert.Empty(t, filters.identityNames)
}

func TestParseFilterFlags_ProvidersWithNames(t *testing.T) {
	cmd := createTestAuthListCmd()
	cmd.Flags().Set("providers", "aws-sso,okta")
	cmd.Flags().Lookup("providers").Changed = true

	filters, err := parseFilterFlags(cmd)

	require.NoError(t, err)
	assert.True(t, filters.showProvidersOnly)
	assert.Equal(t, []string{"aws-sso", "okta"}, filters.providerNames)
}

func TestParseFilterFlags_ProvidersWithSpaces(t *testing.T) {
	cmd := createTestAuthListCmd()
	cmd.Flags().Set("providers", " aws-sso , okta ")
	cmd.Flags().Lookup("providers").Changed = true

	filters, err := parseFilterFlags(cmd)

	require.NoError(t, err)
	assert.True(t, filters.showProvidersOnly)
	assert.Equal(t, []string{"aws-sso", "okta"}, filters.providerNames)
}

func TestParseFilterFlags_IdentitiesOnly(t *testing.T) {
	cmd := createTestAuthListCmd()
	cmd.Flags().Set("identities", "")
	cmd.Flags().Lookup("identities").Changed = true

	filters, err := parseFilterFlags(cmd)

	require.NoError(t, err)
	assert.False(t, filters.showProvidersOnly)
	assert.True(t, filters.showIdentitiesOnly)
	assert.Empty(t, filters.providerNames)
	assert.Empty(t, filters.identityNames)
}

func TestParseFilterFlags_IdentitiesWithNames(t *testing.T) {
	cmd := createTestAuthListCmd()
	cmd.Flags().Set("identities", "admin,dev,prod")
	cmd.Flags().Lookup("identities").Changed = true

	filters, err := parseFilterFlags(cmd)

	require.NoError(t, err)
	assert.True(t, filters.showIdentitiesOnly)
	assert.Equal(t, []string{"admin", "dev", "prod"}, filters.identityNames)
}

func TestParseFilterFlags_MutuallyExclusive(t *testing.T) {
	cmd := createTestAuthListCmd()
	cmd.Flags().Set("providers", "aws-sso")
	cmd.Flags().Set("identities", "admin")
	cmd.Flags().Lookup("providers").Changed = true
	cmd.Flags().Lookup("identities").Changed = true

	_, err := parseFilterFlags(cmd)

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

func TestExecuteAuthListCommand_SuggestsProfilesWhenNoAuthConfigured(t *testing.T) {
	// Setup: Create a temp dir with minimal atmos.yaml (no auth providers/identities)
	// but with profiles that could contain auth config.
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "atmos.yaml")

	configContent := `auth:
  realm: test
  logs:
    level: Debug
`
	err := os.WriteFile(configFile, []byte(configContent), 0o600)
	require.NoError(t, err)

	// Create profile directories (simulating profiles that may contain auth config).
	for _, profileName := range []string{"developers", "devops", "superadmin"} {
		profileDir := filepath.Join(tempDir, "profiles", profileName)
		err := os.MkdirAll(profileDir, 0o755)
		require.NoError(t, err)

		// Add a minimal atmos.yaml in each profile so it's discovered.
		profileConfig := filepath.Join(profileDir, "atmos.yaml")
		err = os.WriteFile(profileConfig, []byte("# profile config\n"), 0o600)
		require.NoError(t, err)
	}

	// Set environment to use test config.
	t.Chdir(tempDir)

	_ = NewTestKit(t)

	cmd := createTestAuthListCmd()
	cmd.RunE = executeAuthListCommand

	// Execute the command - should return an error suggesting profiles.
	err = cmd.Execute()

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthNotConfigured)

	// Verify hints contain profile names and usage suggestion.
	hints := cockroachErrors.GetAllHints(err)
	allHints := strings.Join(hints, " ")
	assert.Contains(t, allHints, "developers")
	assert.Contains(t, allHints, "devops")
	assert.Contains(t, allHints, "superadmin")
	assert.Contains(t, allHints, "--profile")
}

func TestExecuteAuthListCommand_NoErrorWhenNoProfilesExist(t *testing.T) {
	// Setup: Create a temp dir with minimal atmos.yaml (no auth, no profiles).
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "atmos.yaml")

	configContent := `auth:
  realm: test
`
	err := os.WriteFile(configFile, []byte(configContent), 0o600)
	require.NoError(t, err)

	// No profiles directory created.

	t.Chdir(tempDir)

	_ = NewTestKit(t)

	cmd := createTestAuthListCmd()
	cmd.RunE = executeAuthListCommand

	// Execute the command - should NOT return an error (no profiles to suggest).
	err = cmd.Execute()

	assert.NoError(t, err)
}

func TestExecuteAuthListCommand_ProfileFlagSuppressesNoAuthError(t *testing.T) {
	// Regression test: when --profile loads a profile that contains auth config,
	// ErrAuthNotConfigured must NOT be returned, even though the base atmos.yaml
	// has no auth providers/identities.
	tempDir := t.TempDir()

	// Base atmos.yaml: no auth providers/identities.
	baseConfig := filepath.Join(tempDir, "atmos.yaml")
	err := os.WriteFile(baseConfig, []byte(`auth:
  realm: test
`), 0o600)
	require.NoError(t, err)

	// Profile "devops" contains auth providers and identities.
	profileDir := filepath.Join(tempDir, "profiles", "devops")
	require.NoError(t, os.MkdirAll(profileDir, 0o755))
	profileConfig := filepath.Join(profileDir, "atmos.yaml")
	err = os.WriteFile(profileConfig, []byte(`auth:
  providers:
    my-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://example.awsapps.com/start
  identities:
    dev-role:
      kind: aws/permission-set
      provider: my-sso
      permission_set: DevOps
`), 0o600)
	require.NoError(t, err)

	t.Chdir(tempDir)

	_ = NewTestKit(t)

	cmd := createTestAuthListCmd()
	cmd.RunE = executeAuthListCommand

	// Set profile via Viper (matching how global flag binding works in real execution).
	// In production, RootCmd's persistent --profile flag is bound to Viper via BindToViper.
	// loadAuthManagerForList calls flags.ParseGlobalFlags which reads v.GetStringSlice("profile").
	v := viper.GetViper()
	v.Set("profile", []string{"devops"})
	t.Cleanup(func() { v.Set("profile", []string{}) })

	// Execute — should NOT return ErrAuthNotConfigured because the profile
	// contributes auth providers/identities to the merged configuration.
	err = cmd.Execute()
	// The command should succeed (no ErrAuthNotConfigured) or fail for a different
	// reason (e.g., mock provider not available). Either way, it must not be
	// ErrAuthNotConfigured which would mean --profile was ignored.
	if err != nil {
		assert.NotErrorIs(t, err, errUtils.ErrAuthNotConfigured,
			"--profile should load auth config and suppress ErrAuthNotConfigured")
	}
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
