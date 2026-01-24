package auth

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestParseCommaSeparatedNames(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single name",
			input:    "admin",
			expected: []string{"admin"},
		},
		{
			name:     "multiple names",
			input:    "admin,dev,prod",
			expected: []string{"admin", "dev", "prod"},
		},
		{
			name:     "names with spaces",
			input:    "admin , dev , prod",
			expected: []string{"admin", "dev", "prod"},
		},
		{
			name:     "names with leading/trailing spaces",
			input:    "  admin  ,  dev  ",
			expected: []string{"admin", "dev"},
		},
		{
			name:     "empty elements are skipped",
			input:    "admin,,prod",
			expected: []string{"admin", "prod"},
		},
		{
			name:     "only commas",
			input:    ",,",
			expected: []string{},
		},
		{
			name:     "single name with spaces",
			input:    "  admin  ",
			expected: []string{"admin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCommaSeparatedNames(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterProviders(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
		"okta":    {Kind: "saml"},
		"azure":   {Kind: "azure/oidc"},
	}

	// Test nil names returns all providers.
	filtered, identities, err := filterProviders(providers, nil)
	assert.NoError(t, err)
	assert.Len(t, filtered, 3)
	assert.Empty(t, identities)

	// Test empty names returns all providers.
	filtered, identities, err = filterProviders(providers, []string{})
	assert.NoError(t, err)
	assert.Len(t, filtered, 3)
	assert.Empty(t, identities)

	// Test filtering by specific name.
	filtered, _, err = filterProviders(providers, []string{"aws-sso"})
	assert.NoError(t, err)
	assert.Len(t, filtered, 1)
	assert.Contains(t, filtered, "aws-sso")

	// Test filtering by multiple names.
	filtered, _, err = filterProviders(providers, []string{"aws-sso", "okta"})
	assert.NoError(t, err)
	assert.Len(t, filtered, 2)

	// Test non-existent provider returns error.
	_, _, err = filterProviders(providers, []string{"gcp"})
	assert.ErrorIs(t, err, errUtils.ErrProviderNotFound)

	// Test empty map with filter returns error.
	_, _, err = filterProviders(map[string]schema.Provider{}, []string{"any"})
	assert.ErrorIs(t, err, errUtils.ErrProviderNotFound)
}

func TestFilterIdentities(t *testing.T) {
	identities := map[string]schema.Identity{
		"admin": {Kind: "aws/user"},
		"dev":   {Kind: "aws/permission-set"},
	}

	// Test nil names returns all identities.
	providers, filtered, err := filterIdentities(identities, nil)
	assert.NoError(t, err)
	assert.Len(t, filtered, 2)
	assert.Empty(t, providers)

	// Test filtering by specific name.
	_, filtered, err = filterIdentities(identities, []string{"admin"})
	assert.NoError(t, err)
	assert.Len(t, filtered, 1)
	assert.Contains(t, filtered, "admin")

	// Test non-existent identity returns error.
	_, _, err = filterIdentities(identities, []string{"staging"})
	assert.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
}

func TestApplyFilters(t *testing.T) {
	providers := map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
	}
	identities := map[string]schema.Identity{
		"admin": {Kind: "aws/user"},
	}

	tests := []struct {
		name               string
		filters            *filterConfig
		expectedProviders  int
		expectedIdentities int
		expectedError      error
	}{
		{
			name: "no filters - returns both",
			filters: &filterConfig{
				showProvidersOnly:  false,
				showIdentitiesOnly: false,
			},
			expectedProviders:  1,
			expectedIdentities: 1,
			expectedError:      nil,
		},
		{
			name: "providers only",
			filters: &filterConfig{
				showProvidersOnly:  true,
				showIdentitiesOnly: false,
			},
			expectedProviders:  1,
			expectedIdentities: 0,
			expectedError:      nil,
		},
		{
			name: "identities only",
			filters: &filterConfig{
				showProvidersOnly:  false,
				showIdentitiesOnly: true,
			},
			expectedProviders:  0,
			expectedIdentities: 1,
			expectedError:      nil,
		},
		{
			name: "providers with specific name",
			filters: &filterConfig{
				showProvidersOnly: true,
				providerNames:     []string{"aws-sso"},
			},
			expectedProviders:  1,
			expectedIdentities: 0,
			expectedError:      nil,
		},
		{
			name: "providers with non-existent name",
			filters: &filterConfig{
				showProvidersOnly: true,
				providerNames:     []string{"nonexistent"},
			},
			expectedProviders:  0,
			expectedIdentities: 0,
			expectedError:      errUtils.ErrProviderNotFound,
		},
		{
			name: "identities with specific name",
			filters: &filterConfig{
				showIdentitiesOnly: true,
				identityNames:      []string{"admin"},
			},
			expectedProviders:  0,
			expectedIdentities: 1,
			expectedError:      nil,
		},
		{
			name: "identities with non-existent name",
			filters: &filterConfig{
				showIdentitiesOnly: true,
				identityNames:      []string{"nonexistent"},
			},
			expectedProviders:  0,
			expectedIdentities: 0,
			expectedError:      errUtils.ErrIdentityNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultProviders, resultIdentities, err := applyFilters(providers, identities, tt.filters)

			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Len(t, resultProviders, tt.expectedProviders)
				assert.Len(t, resultIdentities, tt.expectedIdentities)
			}
		})
	}
}

func TestRenderJSON(t *testing.T) {
	tests := []struct {
		name       string
		providers  map[string]schema.Provider
		identities map[string]schema.Identity
		checkKeys  []string
	}{
		{
			name:       "empty maps",
			providers:  map[string]schema.Provider{},
			identities: map[string]schema.Identity{},
			checkKeys:  []string{},
		},
		{
			name: "providers only",
			providers: map[string]schema.Provider{
				"aws-sso": {Kind: "aws/iam-identity-center"},
			},
			identities: map[string]schema.Identity{},
			checkKeys:  []string{"providers"},
		},
		{
			name:      "identities only",
			providers: map[string]schema.Provider{},
			identities: map[string]schema.Identity{
				"admin": {Kind: "aws/user"},
			},
			checkKeys: []string{"identities"},
		},
		{
			name: "both providers and identities",
			providers: map[string]schema.Provider{
				"aws-sso": {Kind: "aws/iam-identity-center"},
			},
			identities: map[string]schema.Identity{
				"admin": {Kind: "aws/user"},
			},
			checkKeys: []string{"providers", "identities"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderJSON(tt.providers, tt.identities)
			require.NoError(t, err)

			// Verify it's valid JSON.
			var parsed map[string]interface{}
			err = json.Unmarshal([]byte(result), &parsed)
			require.NoError(t, err, "should produce valid JSON")

			// Verify expected keys are present.
			for _, key := range tt.checkKeys {
				assert.Contains(t, parsed, key)
			}

			// Verify ends with newline.
			assert.True(t, strings.HasSuffix(result, "\n"))
		})
	}
}

func TestRenderYAML(t *testing.T) {
	tests := []struct {
		name       string
		providers  map[string]schema.Provider
		identities map[string]schema.Identity
		checkKeys  []string
	}{
		{
			name:       "empty maps",
			providers:  map[string]schema.Provider{},
			identities: map[string]schema.Identity{},
			checkKeys:  []string{},
		},
		{
			name: "providers only",
			providers: map[string]schema.Provider{
				"aws-sso": {Kind: "aws/iam-identity-center"},
			},
			identities: map[string]schema.Identity{},
			checkKeys:  []string{"providers"},
		},
		{
			name:      "identities only",
			providers: map[string]schema.Provider{},
			identities: map[string]schema.Identity{
				"admin": {Kind: "aws/user"},
			},
			checkKeys: []string{"identities"},
		},
		{
			name: "both providers and identities",
			providers: map[string]schema.Provider{
				"aws-sso": {Kind: "aws/iam-identity-center"},
			},
			identities: map[string]schema.Identity{
				"admin": {Kind: "aws/user"},
			},
			checkKeys: []string{"providers", "identities"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderYAML(tt.providers, tt.identities)
			require.NoError(t, err)

			// Verify it's valid YAML.
			var parsed map[string]interface{}
			err = yaml.Unmarshal([]byte(result), &parsed)
			require.NoError(t, err, "should produce valid YAML")

			// Verify expected keys are present.
			for _, key := range tt.checkKeys {
				assert.Contains(t, parsed, key)
			}
		})
	}
}

func TestFilterConfig(t *testing.T) {
	// Test that filterConfig struct is properly defined.
	cfg := filterConfig{
		showProvidersOnly:  true,
		showIdentitiesOnly: false,
		providerNames:      []string{"aws-sso", "okta"},
		identityNames:      []string{"admin", "dev"},
	}

	assert.True(t, cfg.showProvidersOnly)
	assert.False(t, cfg.showIdentitiesOnly)
	assert.Equal(t, []string{"aws-sso", "okta"}, cfg.providerNames)
	assert.Equal(t, []string{"admin", "dev"}, cfg.identityNames)
}

func TestListFormatFlagCompletion(t *testing.T) {
	completions, directive := listFormatFlagCompletion(nil, nil, "")

	assert.Contains(t, completions, "tree")
	assert.Contains(t, completions, "table")
	assert.Contains(t, completions, "json")
	assert.Contains(t, completions, "yaml")
	assert.Contains(t, completions, "graphviz")
	assert.Contains(t, completions, "mermaid")
	assert.Contains(t, completions, "markdown")
	assert.Equal(t, completions, []string{"tree", "table", "json", "yaml", "graphviz", "mermaid", "markdown"})
	// Check it's a completion directive that doesn't complete files.
	assert.NotZero(t, directive)
}

func TestAuthListCommand_Structure(t *testing.T) {
	assert.Equal(t, "list", authListCmd.Use)
	assert.NotEmpty(t, authListCmd.Short)
	assert.NotEmpty(t, authListCmd.Long)
	assert.NotNil(t, authListCmd.RunE)

	// Check format flag exists.
	formatFlag := authListCmd.Flags().Lookup("format")
	assert.NotNil(t, formatFlag)
	assert.Equal(t, "f", formatFlag.Shorthand)

	// Check providers flag exists.
	providersFlag := authListCmd.Flags().Lookup("providers")
	assert.NotNil(t, providersFlag)

	// Check identities flag exists.
	identitiesFlag := authListCmd.Flags().Lookup("identities")
	assert.NotNil(t, identitiesFlag)
}
