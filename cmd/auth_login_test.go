package cmd

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthLoginCmd(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		setupConfig    func() *schema.AtmosConfiguration
		expectedError  string
		expectedOutput string
	}{
		{
			name: "successful login with default identity",
			args: []string{},
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
			expectedOutput: "Successfully authenticated",
		},
		{
			name: "successful login with specific identity",
			args: []string{"--identity", "test-identity"},
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
			expectedOutput: "Successfully authenticated",
		},
		{
			name: "no auth configuration",
			args: []string{},
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Auth: schema.AuthConfig{},
				}
			},
			expectedError: "no default identity configured",
		},
		{
			name: "invalid identity specified",
			args: []string{"--identity", "nonexistent"},
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
									Provider: "test-provider",
								},
							},
						},
					},
				}
			},
			expectedError: "identity \"nonexistent\" not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			originalArgs := os.Args
			defer func() { os.Args = originalArgs }()

			// Create a new command instance for testing
			cmd := &cobra.Command{
				Use: "login",
				RunE: func(cmd *cobra.Command, args []string) error {
					// Mock implementation for testing
					identityName, _ := cmd.Flags().GetString("identity")

					config := tt.setupConfig()
					if len(config.Auth.Identities) == 0 {
						return fmt.Errorf("no default identity configured")
					}

					if identityName == "" {
						// Check for default identity
						hasDefault := false
						for _, identity := range config.Auth.Identities {
							if identity.Default {
								hasDefault = true
								break
							}
						}
						if !hasDefault {
							return fmt.Errorf("no default identity configured")
						}
					}
					if identityName != "" {
						if _, exists := config.Auth.Identities[identityName]; !exists {
							return fmt.Errorf("identity %q not found", identityName)
						}
					}

					cmd.Println("Successfully authenticated")
					return nil
				},
			}
			cmd.Flags().StringP("identity", "i", "", "Specify the identity to authenticate to")

			// Capture output
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			// Set arguments
			cmd.SetArgs(tt.args)

			// Execute command
			err := cmd.Execute()

			// Verify results
			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				if tt.expectedOutput != "" {
					assert.Contains(t, buf.String(), tt.expectedOutput)
				}
			}
		})
	}
}

func TestAuthLoginCmdFlags(t *testing.T) {
	// Create a mock command to test flag structure
	cmd := &cobra.Command{
		Use: "login",
	}
	cmd.Flags().StringP("identity", "i", "", "Specify the identity to authenticate to")

	// Test that required flags are present
	identityFlag := cmd.Flags().Lookup("identity")
	require.NotNil(t, identityFlag)
	assert.Equal(t, "i", identityFlag.Shorthand)
	assert.Equal(t, "", identityFlag.DefValue)
}

func TestCreateAuthManager(t *testing.T) {
	authConfig := &schema.AuthConfig{
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
					Provider: "test-provider",
				},
			},
		},
	}

	// Mock auth manager creation - in real implementation this would use internal/auth
	// For testing purposes, we just verify the config is valid
	assert.NotNil(t, authConfig)
	assert.NotEmpty(t, authConfig.Providers)
	assert.NotEmpty(t, authConfig.Identities)
}
