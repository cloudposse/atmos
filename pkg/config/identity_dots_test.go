package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestIdentityNamesWithDots verifies that identity names containing dots are correctly parsed.
// When auto-provisioning AWS SSO identities, account names can contain dots (e.g., "product.usa"),
// and these should not be treated as nested paths by Viper.
func TestIdentityNamesWithDots(t *testing.T) {
	// Create temp directory.
	tmpDir := t.TempDir()

	// Create test config with identity names containing dots.
	configPath := filepath.Join(tmpDir, "atmos.yaml")
	configContent := `auth:
  providers:
    test-provider:
      kind: mock
  identities:
    product.usa/ReadOnlyAccess:
      kind: mock
      provider: test-provider
    dev.env/AdminAccess:
      kind: mock
      provider: test-provider
    simple-name:
      kind: mock
      provider: test-provider
`
	err := os.WriteFile(configPath, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Load config.
	configAndStacksInfo := &schema.ConfigAndStacksInfo{
		AtmosConfigFilesFromArg: []string{configPath},
	}
	config, err := LoadConfig(configAndStacksInfo)
	require.NoError(t, err)

	// Verify all three identities are loaded correctly.
	assert.Len(t, config.Auth.Identities, 3, "Should have exactly 3 identities")

	// Check identity with dot and slash.
	identity1, exists := config.Auth.Identities["product.usa/readonlyaccess"]
	assert.True(t, exists, "Identity 'product.usa/ReadOnlyAccess' should exist (lowercase)")
	assert.Equal(t, "mock", identity1.Kind)
	assert.Equal(t, "test-provider", identity1.Provider)

	// Check second identity with dot and slash.
	identity2, exists := config.Auth.Identities["dev.env/adminaccess"]
	assert.True(t, exists, "Identity 'dev.env/AdminAccess' should exist (lowercase)")
	assert.Equal(t, "mock", identity2.Kind)
	assert.Equal(t, "test-provider", identity2.Provider)

	// Check simple name without dots.
	identity3, exists := config.Auth.Identities["simple-name"]
	assert.True(t, exists, "Identity 'simple-name' should exist")
	assert.Equal(t, "mock", identity3.Kind)
	assert.Equal(t, "test-provider", identity3.Provider)

	// Verify case map preserves original case.
	assert.NotNil(t, config.Auth.IdentityCaseMap, "IdentityCaseMap should be populated")
	assert.Equal(t, "product.usa/ReadOnlyAccess", config.Auth.IdentityCaseMap["product.usa/readonlyaccess"])
	assert.Equal(t, "dev.env/AdminAccess", config.Auth.IdentityCaseMap["dev.env/adminaccess"])
	assert.Equal(t, "simple-name", config.Auth.IdentityCaseMap["simple-name"])
}

// TestIdentityNamesWithDotsErrorHandling tests that fixAuthIdentities gracefully handles edge cases.
func TestIdentityNamesWithDotsErrorHandling(t *testing.T) {
	tests := []struct {
		name              string
		configContent     string
		expectedIdentities int
	}{
		{
			name: "identity with scalar value is skipped",
			configContent: `auth:
  providers:
    test-provider:
      kind: mock
  identities:
    product.usa/ReadOnlyAccess: "invalid-string-value"
    valid.identity/Access:
      kind: mock
      provider: test-provider
`,
			// Viper will still parse the scalar incorrectly, but fixAuthIdentities will skip it.
			// The valid identity should be processed.
			expectedIdentities: 1,
		},
		{
			name: "identity with list value is skipped",
			configContent: `auth:
  providers:
    test-provider:
      kind: mock
  identities:
    product.usa/ReadOnlyAccess:
      - invalid
      - list
    valid.identity/Access:
      kind: mock
      provider: test-provider
`,
			// The list-valued identity will be skipped, valid one processed.
			expectedIdentities: 1,
		},
		{
			name: "config with no auth section",
			configContent: `some_other:
  config: value
`,
			expectedIdentities: 0,
		},
		{
			name: "auth without identities section",
			configContent: `auth:
  providers:
    test-provider:
      kind: mock
`,
			expectedIdentities: 0,
		},
		{
			name: "empty identities section",
			configContent: `auth:
  providers:
    test-provider:
      kind: mock
  identities: {}
`,
			expectedIdentities: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "atmos.yaml")
			err := os.WriteFile(configPath, []byte(tt.configContent), 0o644)
			require.NoError(t, err)

			// Load config.
			configAndStacksInfo := &schema.ConfigAndStacksInfo{
				AtmosConfigFilesFromArg: []string{configPath},
			}
			config, err := LoadConfig(configAndStacksInfo)
			require.NoError(t, err, "Config loading should not fail")

			// Check that the expected number of identities are loaded.
			assert.Len(t, config.Auth.Identities, tt.expectedIdentities, 
				"Should have %d identities", tt.expectedIdentities)
		})
	}
}
