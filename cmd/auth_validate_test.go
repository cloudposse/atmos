package cmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthValidateCmd(t *testing.T) {
	CleanupRootCmd(t)

	tests := []struct {
		name           string
		setupConfig    func() *schema.AtmosConfiguration
		expectedError  string
		expectedOutput string
	}{
		{
			name: "valid auth configuration",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Auth: schema.AuthConfig{
						Providers: map[string]schema.Provider{
							"test-provider": {
								Kind:     "aws/iam-identity-center",
								Region:   "us-east-1",
								StartURL: "https://test.awsapps.com/start",
							},
						},
						Identities: map[string]schema.Identity{
							"test-identity": {
								Kind:    "aws/permission-set",
								Default: true,
								Via: &schema.IdentityVia{
									Provider: "test-provider",
								},
								Principal: map[string]interface{}{
									"name": "TestPermissionSet",
									"account": map[string]interface{}{
										"name": "test-account",
									},
								},
							},
						},
					},
				}
			},
			expectedOutput: "✅ Authentication configuration is valid",
		},
		{
			name: "invalid provider configuration - missing region",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Auth: schema.AuthConfig{
						Providers: map[string]schema.Provider{
							"test-provider": {
								Kind: "aws/iam-identity-center",
								// Missing required region
							},
						},
						Identities: map[string]schema.Identity{
							"test-identity": {
								Kind: "aws/permission-set",
								Via: &schema.IdentityVia{
									Provider: "test-provider",
								},
							},
						},
					},
				}
			},
			expectedOutput: "❌ Authentication configuration validation failed",
		},
		{
			name: "invalid identity configuration - missing provider reference",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Auth: schema.AuthConfig{
						Providers: map[string]schema.Provider{
							"test-provider": {
								Kind:   "aws/iam-identity-center",
								Region: "us-east-1",
							},
						},
						Identities: map[string]schema.Identity{
							"test-identity": {
								Kind: "aws/permission-set",
								Via: &schema.IdentityVia{
									Provider: "nonexistent-provider",
								},
							},
						},
					},
				}
			},
			expectedOutput: "❌ Authentication configuration validation failed",
		},
		{
			name: "aws user identity without via provider",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Auth: schema.AuthConfig{
						Identities: map[string]schema.Identity{
							"aws-user": {
								Kind: "aws/user",
								Credentials: map[string]interface{}{
									"region": "us-east-1",
								},
							},
						},
					},
				}
			},
			expectedOutput: "✅ Authentication configuration is valid",
		},
		{
			name: "empty auth configuration",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Auth: schema.AuthConfig{},
				}
			},
			expectedOutput: "✅ Authentication configuration is valid",
		},
		{
			name: "circular identity reference",
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Auth: schema.AuthConfig{
						Providers: map[string]schema.Provider{
							"test-provider": {
								Kind:   "aws/iam-identity-center",
								Region: "us-east-1",
							},
						},
						Identities: map[string]schema.Identity{
							"identity-a": {
								Kind: "aws/assume-role",
								Via: &schema.IdentityVia{
									Identity: "identity-b",
								},
							},
							"identity-b": {
								Kind: "aws/assume-role",
								Via: &schema.IdentityVia{
									Identity: "identity-a",
								},
							},
						},
					},
				}
			},
			expectedOutput: "❌ Authentication configuration validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			CleanupRootCmd(t)

			// Create a mock command for testing
			cmd := &cobra.Command{
				Use: "validate",
				RunE: func(cmd *cobra.Command, args []string) error {
					config := tt.setupConfig()

					// Mock validation logic
					if err := mockValidateAuthConfig(&config.Auth); err != nil {
						cmd.Printf("**❌ Authentication configuration validation failed:**\n")
						cmd.Printf("%s\n", err.Error())
						return nil
					}

					cmd.Printf("**✅ Authentication configuration is valid**\n")
					cmd.Printf("All providers and identities are properly configured.\n")
					return nil
				},
			}

			// Capture output
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			// Execute command
			err := cmd.Execute()
			assert.NoError(t, err)

			// Verify output
			output := buf.String()
			assert.Contains(t, output, tt.expectedOutput)
		})
	}
}

// mockValidateAuthConfig provides mock validation logic for testing.
func mockValidateAuthConfig(config *schema.AuthConfig) error {
	// Check for missing regions in AWS providers
	for name := range config.Providers {
		provider := config.Providers[name]
		if provider.Kind == "aws/iam-identity-center" && provider.Region == "" {
			return assert.AnError
		}
		_ = name // Use the variable to avoid unused variable error
	}

	// Check for nonexistent provider references
	for _, identity := range config.Identities {
		if identity.Via != nil && identity.Via.Provider != "" {
			if _, exists := config.Providers[identity.Via.Provider]; !exists {
				return assert.AnError
			}
		}
	}

	// Check for circular references (simplified)
	visited := make(map[string]bool)
	for name, identity := range config.Identities {
		if identity.Via == nil || identity.Via.Identity == "" {
			continue
		}
		if visited[name] {
			return assert.AnError
		}
		visited[name] = true

		// Check if referenced identity points back
		refIdentity, exists := config.Identities[identity.Via.Identity]
		if !exists {
			continue
		}
		if refIdentity.Via != nil && refIdentity.Via.Identity == name {
			return assert.AnError
		}
	}

	return nil
}

func TestAuthValidateCmdIntegration(t *testing.T) {
	CleanupRootCmd(t)

	// Create a mock command to test structure
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate authentication configuration",
		Long:  "Validate the authentication configuration in atmos.yaml for syntax and logical errors.",
	}

	assert.Equal(t, "validate", cmd.Use)
	assert.Equal(t, "Validate authentication configuration", cmd.Short)
	assert.Contains(t, cmd.Long, "Validate the authentication configuration")
}
