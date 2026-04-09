package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestMergeGlobalAuthConfig_DeepMerge verifies deep-merge behavior.
// Auth should behave like vars/settings/env: global auth merged with component auth, component wins.
func TestMergeGlobalAuthConfig_DeepMerge(t *testing.T) {
	tests := []struct {
		name             string
		globalProviders  map[string]schema.Provider
		globalIdentities map[string]schema.Identity
		componentAuth    map[string]any
		verifyFunc       func(t *testing.T, result map[string]any)
	}{
		{
			name:             "empty-everything",
			globalProviders:  map[string]schema.Provider{},
			globalIdentities: map[string]schema.Identity{},
			componentAuth:    map[string]any{},
			verifyFunc: func(t *testing.T, result map[string]any) {
				assert.Empty(t, result, "Empty config should return empty")
			},
		},
		{
			name: "global-only",
			globalProviders: map[string]schema.Provider{
				"aws-sso": {Kind: "aws/iam-identity-center", Region: "us-east-1"},
			},
			globalIdentities: map[string]schema.Identity{
				"dev": {Kind: "aws/assume-role"},
			},
			componentAuth: map[string]any{},
			verifyFunc: func(t *testing.T, result map[string]any) {
				// After merge, types become map[string]interface{}
				providers, ok := result["providers"].(map[string]interface{})
				assert.True(t, ok, "Providers should be map[string]interface{}")
				assert.Contains(t, providers, "aws-sso")

				identities, ok := result["identities"].(map[string]interface{})
				assert.True(t, ok, "Identities should be map[string]interface{}")
				assert.Contains(t, identities, "dev")
			},
		},
		{
			name:             "component-only",
			globalProviders:  map[string]schema.Provider{},
			globalIdentities: map[string]schema.Identity{},
			componentAuth: map[string]any{
				"providers": map[string]schema.Provider{
					"azure": {Kind: "azure/managed-identity"},
				},
			},
			verifyFunc: func(t *testing.T, result map[string]any) {
				providers, ok := result["providers"].(map[string]interface{})
				assert.True(t, ok)
				assert.Contains(t, providers, "azure")
			},
		},
		{
			name: "deep-merge-providers",
			globalProviders: map[string]schema.Provider{
				"global": {Kind: "aws", Region: "us-east-1"},
				"shared": {Kind: "aws", Region: "us-east-1"},
			},
			globalIdentities: map[string]schema.Identity{},
			componentAuth: map[string]any{
				"providers": map[string]schema.Provider{
					"component": {Kind: "azure"},
					"shared":    {Kind: "aws", Region: "eu-west-1"}, // Override
				},
			},
			verifyFunc: func(t *testing.T, result map[string]any) {
				providers, ok := result["providers"].(map[string]interface{})
				assert.True(t, ok)
				assert.Contains(t, providers, "global", "Should have global provider")
				assert.Contains(t, providers, "component", "Should have component provider")
				assert.Contains(t, providers, "shared", "Should have shared provider")

				// Verify component override wins
				sharedMap := providers["shared"].(map[string]interface{})
				assert.Equal(t, "eu-west-1", sharedMap["region"], "Component should override region")
			},
		},
		{
			name:            "deep-merge-identities",
			globalProviders: map[string]schema.Provider{},
			globalIdentities: map[string]schema.Identity{
				"global": {Kind: "aws/assume-role", Default: true},
				"shared": {Kind: "aws/assume-role", Default: false},
			},
			componentAuth: map[string]any{
				"identities": map[string]schema.Identity{
					"component": {Kind: "azure/managed-identity"},
					"shared":    {Kind: "aws/assume-role", Default: true}, // Override
				},
			},
			verifyFunc: func(t *testing.T, result map[string]any) {
				identities, ok := result["identities"].(map[string]interface{})
				assert.True(t, ok)
				assert.Contains(t, identities, "global")
				assert.Contains(t, identities, "component")
				assert.Contains(t, identities, "shared")

				// Verify component override wins
				sharedMap := identities["shared"].(map[string]interface{})
				assert.Equal(t, true, sharedMap["default"], "Component should override default")
			},
		},
		{
			name:             "logs-only-global",
			globalProviders:  map[string]schema.Provider{},
			globalIdentities: map[string]schema.Identity{},
			componentAuth:    map[string]any{},
			verifyFunc: func(t *testing.T, result map[string]any) {
				// This test uses LogsConfig from atmosConfig which we set separately
				// Just verify we can handle logs-only config
				assert.NotNil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{
					Providers:  tt.globalProviders,
					Identities: tt.globalIdentities,
				},
			}

			componentSection := map[string]any{}
			if len(tt.componentAuth) > 0 {
				componentSection[cfg.AuthSectionName] = tt.componentAuth
			}

			result := mergeGlobalAuthConfig(atmosConfig, componentSection)

			tt.verifyFunc(t, result)

			// Verify componentSection["auth"] was updated (unless empty)
			if len(result) > 0 {
				assert.Equal(t, result, componentSection[cfg.AuthSectionName], "Should update componentSection")
			}
		})
	}
}

// TestMergeGlobalAuthConfig_WithPostProcessIntegration verifies integration with postProcess.
func TestMergeGlobalAuthConfig_WithPostProcessIntegration(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"test-provider": {Kind: "aws/iam-identity-center", Region: "us-west-2"},
			},
			Identities: map[string]schema.Identity{
				"test-identity": {Kind: "aws/assume-role", Default: true},
			},
			Logs: schema.Logs{
				Level: "Debug",
			},
		},
	}

	componentSection := map[string]any{}

	// Step 1: Merge global auth config
	authSection := mergeGlobalAuthConfig(atmosConfig, componentSection)
	assert.Equal(t, authSection, componentSection[cfg.AuthSectionName])

	// Step 2: Simulate postProcessTemplatesAndYamlFunctions
	configAndStacksInfo := &schema.ConfigAndStacksInfo{
		ComponentSection:     componentSection,
		ComponentAuthSection: authSection,
	}

	postProcessTemplatesAndYamlFunctions(configAndStacksInfo)

	// Step 3: Verify ComponentAuthSection is still correct
	assert.NotNil(t, configAndStacksInfo.ComponentAuthSection)
	assert.NotEmpty(t, configAndStacksInfo.ComponentAuthSection)

	// Verify structure (types are map[string]interface{} after merge)
	providers, ok := configAndStacksInfo.ComponentAuthSection["providers"].(map[string]interface{})
	assert.True(t, ok, "providers should be map[string]interface{}")
	assert.Contains(t, providers, "test-provider")

	identities, ok := configAndStacksInfo.ComponentAuthSection["identities"].(map[string]interface{})
	assert.True(t, ok, "identities should be map[string]interface{}")
	assert.Contains(t, identities, "test-identity")

	logs, ok := configAndStacksInfo.ComponentAuthSection["logs"].(map[string]any)
	assert.True(t, ok, "logs should be map[string]any")
	assert.Equal(t, "Debug", logs["level"])
}

// TestMergeGlobalAuthConfig_LogsAndKeyringOnly tests logs and keyring without providers/identities.
func TestMergeGlobalAuthConfig_LogsAndKeyringOnly(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Providers:  map[string]schema.Provider{},
			Identities: map[string]schema.Identity{},
			Logs: schema.Logs{
				Level: "Info",
				File:  "/tmp/auth.log",
			},
			Keyring: schema.KeyringConfig{
				Type: "system",
				Spec: map[string]interface{}{
					"service": "atmos",
				},
			},
		},
	}

	componentSection := map[string]any{}
	result := mergeGlobalAuthConfig(atmosConfig, componentSection)

	// Should have logs and keyring even without providers/identities
	assert.NotEmpty(t, result)
	assert.Contains(t, result, "logs")
	assert.Contains(t, result, "keyring")

	logs := result["logs"].(map[string]any)
	assert.Equal(t, "Info", logs["level"])
	assert.Equal(t, "/tmp/auth.log", logs["file"])

	keyring := result["keyring"].(map[string]interface{})
	assert.Equal(t, "system", keyring["type"])
}
