package auth

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

// TestParseFilterFlags covers mutual-exclusivity validation and the comma-
// separated parsing branches of parseFilterFlags.
func TestParseFilterFlags(t *testing.T) {
	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "list"}
		cmd.Flags().String("providers", "", "providers")
		cmd.Flags().String("identities", "", "identities")
		return cmd
	}

	t.Run("neither flag changed returns default config", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		cmd := newCmd()

		got, err := parseFilterFlags(cmd)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.False(t, got.showProvidersOnly)
		assert.False(t, got.showIdentitiesOnly)
		assert.Empty(t, got.providerNames)
		assert.Empty(t, got.identityNames)
	})

	t.Run("--providers without value enables providers-only filter", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("providers", ""))

		got, err := parseFilterFlags(cmd)
		require.NoError(t, err)
		assert.True(t, got.showProvidersOnly)
		assert.Empty(t, got.providerNames)
	})

	t.Run("--providers with comma list parses names", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		viper.Set(providersKey, "p1, p2 , p3")

		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("providers", "p1, p2 , p3"))

		got, err := parseFilterFlags(cmd)
		require.NoError(t, err)
		assert.True(t, got.showProvidersOnly)
		assert.Equal(t, []string{"p1", "p2", "p3"}, got.providerNames,
			"comma-separated values must be split and trimmed")
	})

	t.Run("--identities with comma list parses names", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		viper.Set(identitiesKey, "i1,i2")

		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("identities", "i1,i2"))

		got, err := parseFilterFlags(cmd)
		require.NoError(t, err)
		assert.True(t, got.showIdentitiesOnly)
		assert.Equal(t, []string{"i1", "i2"}, got.identityNames)
	})

	t.Run("both --providers and --identities is mutually-exclusive error", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("providers", "p1"))
		require.NoError(t, cmd.Flags().Set("identities", "i1"))

		got, err := parseFilterFlags(cmd)
		require.Error(t, err)
		assert.Nil(t, got)
		assert.ErrorIs(t, err, errUtils.ErrMutuallyExclusiveFlags)
	})
}

// TestRenderOutput_InvalidFormatErrors guards the default-case error in the
// dispatcher. Each valid format branch is covered by other tests; this test
// just pins the negative path.
func TestRenderOutput_InvalidFormatErrors(t *testing.T) {
	got, err := renderOutput(nil, nil, nil, "no-such-format")
	require.Error(t, err)
	assert.Empty(t, got)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
}

// TestRenderOutput_AllValidFormats covers each format branch in the dispatcher
// with empty maps. The intent is branch coverage of the switch statement,
// not assertions on the renderers' output (those are tested in pkg/auth/list).
func TestRenderOutput_AllValidFormats(t *testing.T) {
	providers := map[string]schema.Provider{}
	identities := map[string]schema.Identity{}

	formats := []string{"table", "tree", "json", "yaml", "graphviz", "dot", "mermaid", "markdown", "md"}
	for _, format := range formats {
		t.Run("format="+format, func(t *testing.T) {
			// Most renderers tolerate empty input and return an empty string;
			// the contract here is "no panic + no error from the dispatcher".
			out, err := renderOutput(nil, providers, identities, format)
			// Some renderers may error on nil authManager — both outcomes are
			// acceptable for branch coverage; we just want the switch covered.
			if err == nil {
				assert.NotNil(t, out, "successful render must return a non-nil string")
			}
		})
	}
}

// TestListFlagCompletions_NoConfig covers the early-return path of
// listProvidersFlagCompletion / listIdentitiesFlagCompletion when
// cfg.InitCliConfig fails (no atmos.yaml). They must return nil, no panic.
func TestListFlagCompletions_NoConfig(t *testing.T) {
	// Run from /tmp to ensure no atmos.yaml is found on most machines.
	tmp := t.TempDir()
	t.Chdir(tmp)

	// BuildConfigAndStacksInfo dereferences cmd to read global flags, so we
	// pass a real cobra.Command. It needs no flags registered — completion
	// must remain robust to missing global flags.
	cmd := &cobra.Command{Use: "list-completion-test"}

	// These functions should return safely even without a config.
	t.Run("listProvidersFlagCompletion", func(t *testing.T) {
		assert.NotPanics(t, func() {
			_, _ = listProvidersFlagCompletion(cmd, nil, "")
		})
	})

	t.Run("listIdentitiesFlagCompletion", func(t *testing.T) {
		assert.NotPanics(t, func() {
			_, _ = listIdentitiesFlagCompletion(cmd, nil, "")
		})
	})
}

// TestLoadAuthManagerForList_SmokeFromEmptyTempDir exercises the helper from a
// directory without an atmos.yaml. Either succeeds (defaults loaded) or
// fails with the documented ErrInvalidAuthConfig sentinel.
func TestLoadAuthManagerForList_SmokeFromEmptyTempDir(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	cmd := &cobra.Command{Use: "list"}
	v := viper.New()

	manager, atmosCfg, err := loadAuthManagerForList(cmd, v)
	if err != nil {
		assert.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig,
			"loadAuthManagerForList must wrap failures with ErrInvalidAuthConfig")
		assert.Nil(t, manager)
		assert.Nil(t, atmosCfg)
		return
	}
	assert.NotNil(t, manager)
	assert.NotNil(t, atmosCfg)
}

// TestExecuteAuthListCommand_SmokeNoConfig exercises the list orchestrator
// from a directory without an atmos.yaml. Contract: no panic.
func TestExecuteAuthListCommand_SmokeNoConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	cmd := authListCmd
	cmd.SetContext(context.Background())

	assert.NotPanics(t, func() {
		_ = executeAuthListCommand(cmd, nil)
	})
}

// TestExecuteAuthListCommand_WithMockAuth exercises list end-to-end against
// the mock auth fixture. Drives the load → filter → render-tree → print
// pipeline for non-trivial coverage of list.go.
func TestExecuteAuthListCommand_WithMockAuth(t *testing.T) {
	setupMockAuthFixture(t)

	cmd := authListCmd
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags(nil))

	err := executeAuthListCommand(cmd, nil)
	assert.NoError(t, err)
}

// TestExecuteAuthListCommand_JSONFormat exercises the renderJSON path via
// the orchestrator (this is a coarser-grained check than the unit test
// on renderJSON itself).
func TestExecuteAuthListCommand_JSONFormat(t *testing.T) {
	setupMockAuthFixture(t)

	cmd := authListCmd
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags([]string{"--format=json"}))

	err := executeAuthListCommand(cmd, nil)
	assert.NoError(t, err)
}

// TestSuggestProfilesForAuth_NoProfilesReturnsNil verifies that when no
// profiles exist (empty atmos config), suggestProfilesForAuth returns nil
// (no enrichment) rather than synthesizing an error.
func TestSuggestProfilesForAuth_NoProfilesReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	got := suggestProfilesForAuth(&schema.AtmosConfiguration{})
	assert.Nil(t, got,
		"with no profiles discovered, suggestProfilesForAuth must return nil so the caller can surface its own error")
}
