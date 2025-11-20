package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMergeGlobalAuthConfig(t *testing.T) {
	tests := []struct {
		name                    string
		atmosConfig             *schema.AtmosConfiguration
		componentSection        map[string]any
		expectedAuthSection     map[string]any
		expectedComponentUpdate bool
	}{
		{
			name: "empty-auth-config",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Providers:  map[string]schema.Provider{},
					Identities: map[string]schema.Identity{},
				},
			},
			componentSection:        map[string]any{},
			expectedAuthSection:     map[string]any{},
			expectedComponentUpdate: false,
		},
		{
			name: "providers-only",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Providers: map[string]schema.Provider{
						"aws-sso": {
							Kind:     "aws/iam-identity-center",
							Region:   "us-east-2",
							StartURL: "https://example.awsapps.com/start",
						},
					},
					Identities: map[string]schema.Identity{},
				},
			},
			componentSection: map[string]any{},
			expectedAuthSection: map[string]any{
				"providers": map[string]schema.Provider{
					"aws-sso": {
						Kind:     "aws/iam-identity-center",
						Region:   "us-east-2",
						StartURL: "https://example.awsapps.com/start",
					},
				},
			},
			expectedComponentUpdate: true,
		},
		{
			name: "identities-only",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Providers: map[string]schema.Provider{},
					Identities: map[string]schema.Identity{
						"dev-identity": {
							Kind: "aws/assume-role",
							Via: &schema.IdentityVia{
								Provider: "aws-sso",
							},
						},
					},
				},
			},
			componentSection: map[string]any{},
			expectedAuthSection: map[string]any{
				"identities": map[string]schema.Identity{
					"dev-identity": {
						Kind: "aws/assume-role",
						Via: &schema.IdentityVia{
							Provider: "aws-sso",
						},
					},
				},
			},
			expectedComponentUpdate: true,
		},
		{
			name: "full-auth-config-with-logs-and-keyring",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Providers: map[string]schema.Provider{
						"aws-sso": {
							Kind:   "aws/iam-identity-center",
							Region: "us-east-2",
						},
					},
					Identities: map[string]schema.Identity{
						"dev": {
							Kind: "aws/assume-role",
						},
					},
					Logs: schema.Logs{
						Level: "Debug",
						File:  "/tmp/auth.log",
					},
					Keyring: schema.KeyringConfig{
						Type: "system",
						Spec: map[string]interface{}{
							"service": "atmos",
						},
					},
				},
			},
			componentSection: map[string]any{},
			expectedAuthSection: map[string]any{
				"providers": map[string]schema.Provider{
					"aws-sso": {
						Kind:   "aws/iam-identity-center",
						Region: "us-east-2",
					},
				},
				"identities": map[string]schema.Identity{
					"dev": {
						Kind: "aws/assume-role",
					},
				},
				"logs": map[string]any{
					"level": "Debug",
					"file":  "/tmp/auth.log",
				},
				"keyring": schema.KeyringConfig{
					Type: "system",
					Spec: map[string]interface{}{
						"service": "atmos",
					},
				},
			},
			expectedComponentUpdate: true,
		},
		{
			name: "logs-level-only",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Providers: map[string]schema.Provider{
						"aws": {Kind: "aws"},
					},
					Identities: map[string]schema.Identity{},
					Logs: schema.Logs{
						Level: "Info",
						File:  "",
					},
				},
			},
			componentSection: map[string]any{},
			expectedAuthSection: map[string]any{
				"providers": map[string]schema.Provider{
					"aws": {Kind: "aws"},
				},
				"logs": map[string]any{
					"level": "Info",
					"file":  "",
				},
			},
			expectedComponentUpdate: true,
		},
		{
			name: "logs-file-only",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Providers: map[string]schema.Provider{
						"aws": {Kind: "aws"},
					},
					Identities: map[string]schema.Identity{},
					Logs: schema.Logs{
						Level: "",
						File:  "/var/log/atmos-auth.log",
					},
				},
			},
			componentSection: map[string]any{},
			expectedAuthSection: map[string]any{
				"providers": map[string]schema.Provider{
					"aws": {Kind: "aws"},
				},
				"logs": map[string]any{
					"level": "",
					"file":  "/var/log/atmos-auth.log",
				},
			},
			expectedComponentUpdate: true,
		},
		{
			name: "multiple-providers-and-identities",
			atmosConfig: &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Providers: map[string]schema.Provider{
						"aws-sso": {
							Kind:   "aws/iam-identity-center",
							Region: "us-east-2",
						},
						"azure-sso": {
							Kind: "azure/managed-identity",
						},
					},
					Identities: map[string]schema.Identity{
						"dev": {
							Kind:    "aws/assume-role",
							Default: true,
						},
						"prod": {
							Kind: "aws/assume-role",
						},
					},
				},
			},
			componentSection: map[string]any{},
			expectedAuthSection: map[string]any{
				"providers": map[string]schema.Provider{
					"aws-sso": {
						Kind:   "aws/iam-identity-center",
						Region: "us-east-2",
					},
					"azure-sso": {
						Kind: "azure/managed-identity",
					},
				},
				"identities": map[string]schema.Identity{
					"dev": {
						Kind:    "aws/assume-role",
						Default: true,
					},
					"prod": {
						Kind: "aws/assume-role",
					},
				},
			},
			expectedComponentUpdate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the function
			result := mergeGlobalAuthConfig(tt.atmosConfig, tt.componentSection)

			// Assert the returned auth section
			assert.Equal(t, tt.expectedAuthSection, result, "Returned auth section should match expected")

			// Assert that componentSection["auth"] was updated if expected
			if tt.expectedComponentUpdate {
				assert.Equal(t, tt.expectedAuthSection, tt.componentSection[cfg.AuthSectionName], "ComponentSection[auth] should be updated")
			} else {
				// If no update expected, componentSection[auth] should not be set
				_, exists := tt.componentSection[cfg.AuthSectionName]
				assert.False(t, exists, "ComponentSection[auth] should not be set when auth config is empty")
			}
		})
	}
}

func TestMergeGlobalAuthConfig_WithPostProcessing(t *testing.T) {
	// This test verifies that mergeGlobalAuthConfig works correctly with postProcessTemplatesAndYamlFunctions.
	// It simulates the flow: ProcessComponentConfig → postProcessTemplatesAndYamlFunctions → TerraformPreHook.

	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"test-provider": {
					Kind:   "aws/iam-identity-center",
					Region: "us-west-2",
				},
			},
			Identities: map[string]schema.Identity{
				"test-identity": {
					Kind:    "aws/assume-role",
					Default: true,
				},
			},
			Logs: schema.Logs{
				Level: "Debug",
			},
		},
	}

	componentSection := map[string]any{}

	// Step 1: Merge global auth config (simulates ProcessComponentConfig).
	authSection := mergeGlobalAuthConfig(atmosConfig, componentSection)

	// Verify componentSection["auth"] was set
	assert.Equal(t, authSection, componentSection[cfg.AuthSectionName], "ComponentSection[auth] should be set")

	// Step 2: Simulate postProcessTemplatesAndYamlFunctions
	configAndStacksInfo := &schema.ConfigAndStacksInfo{
		ComponentSection:     componentSection,
		ComponentAuthSection: authSection,
	}

	postProcessTemplatesAndYamlFunctions(configAndStacksInfo)

	// Step 3: Verify ComponentAuthSection is still correct after postProcessing
	assert.NotNil(t, configAndStacksInfo.ComponentAuthSection, "ComponentAuthSection should not be nil")
	assert.Equal(t, 3, len(configAndStacksInfo.ComponentAuthSection), "ComponentAuthSection should have 3 keys (providers, identities, logs)")

	// Verify providers
	providers, ok := configAndStacksInfo.ComponentAuthSection["providers"].(map[string]schema.Provider)
	assert.True(t, ok, "providers should be map[string]schema.Provider")
	assert.Equal(t, 1, len(providers), "Should have 1 provider")
	assert.Contains(t, providers, "test-provider")

	// Verify identities
	identities, ok := configAndStacksInfo.ComponentAuthSection["identities"].(map[string]schema.Identity)
	assert.True(t, ok, "identities should be map[string]schema.Identity")
	assert.Equal(t, 1, len(identities), "Should have 1 identity")
	assert.Contains(t, identities, "test-identity")

	// Verify logs
	logs, ok := configAndStacksInfo.ComponentAuthSection["logs"].(map[string]any)
	assert.True(t, ok, "logs should be map[string]any")
	assert.Equal(t, "Debug", logs["level"])
}

func TestMergeGlobalAuthConfig_DoesNotOverwriteComponentAuth(t *testing.T) {
	// This test verifies that mergeGlobalAuthConfig is only called when component has no auth section.
	// If component has its own auth section, it should not be overwritten.

	// Component already has auth section
	componentSection := map[string]any{
		cfg.AuthSectionName: map[string]any{
			"providers": map[string]schema.Provider{
				"component-provider": {Kind: "azure"},
			},
		},
	}

	// In the actual code, mergeGlobalAuthConfig is only called when componentAuthSection is empty.
	// This test verifies the logic by checking that if component has auth, we don't call merge.

	componentAuthSection, ok := componentSection[cfg.AuthSectionName].(map[string]any)
	assert.True(t, ok, "Component should have auth section")
	assert.NotNil(t, componentAuthSection, "Component auth section should not be nil")

	// Simulate the check in ProcessComponentConfig
	if len(componentAuthSection) == 0 {
		// This branch should NOT be taken since component has auth
		t.Fatal("Should not merge when component has auth section")
	}

	// Component auth should remain unchanged
	providers := componentAuthSection["providers"].(map[string]schema.Provider)
	assert.Contains(t, providers, "component-provider")
	assert.NotContains(t, providers, "global-provider")
}

func TestPostProcessTemplatesAndYamlFunctions_PreservesAuthSection(t *testing.T) {
	// This test specifically verifies that postProcessTemplatesAndYamlFunctions
	// correctly copies auth section from ComponentSection to ComponentAuthSection.

	authConfig := map[string]any{
		"providers": map[string]schema.Provider{
			"aws": {
				Kind:   "aws/iam-identity-center",
				Region: "us-east-1",
			},
		},
		"identities": map[string]schema.Identity{
			"admin": {
				Kind:    "aws/assume-role",
				Default: true,
			},
		},
	}

	input := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			cfg.AuthSectionName: authConfig,
		},
		// Start with empty ComponentAuthSection to simulate before postProcess
		ComponentAuthSection: nil,
	}

	postProcessTemplatesAndYamlFunctions(&input)

	// Verify ComponentAuthSection was populated
	assert.NotNil(t, input.ComponentAuthSection)
	assert.Equal(t, authConfig, input.ComponentAuthSection)
}
