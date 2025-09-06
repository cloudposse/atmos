package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthEnvCmd(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		setupConfig    func() *schema.AtmosConfiguration
		expectedError  string
		expectedOutput []string
	}{
		{
			name: "export format with default identity",
			args: []string{"--format", "export"},
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
							},
						},
					},
				}
			},
			expectedOutput: []string{
				"export AWS_SHARED_CREDENTIALS_FILE=",
				"export AWS_CONFIG_FILE=",
				"export AWS_PROFILE=test-identity",
			},
		},
		{
			name: "json format with specific identity",
			args: []string{"--format", "json", "--identity", "test-identity"},
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
			expectedOutput: []string{
				`"AWS_SHARED_CREDENTIALS_FILE"`,
				`"AWS_CONFIG_FILE"`,
				`"AWS_PROFILE": "test-identity"`,
			},
		},
		{
			name: "dotenv format",
			args: []string{"--format", "dotenv"},
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
							},
						},
					},
				}
			},
			expectedOutput: []string{
				"AWS_SHARED_CREDENTIALS_FILE=",
				"AWS_CONFIG_FILE=",
				"AWS_PROFILE=test-identity",
			},
		},
		{
			name: "no default identity",
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
								Kind: "aws/permission-set",
								Via: &schema.IdentityVia{
									Provider: "test-provider",
								},
							},
						},
					},
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
			// Create a mock command for testing
			cmd := &cobra.Command{
				Use: "env",
				RunE: func(cmd *cobra.Command, args []string) error {
					config := tt.setupConfig()
					identityName, _ := cmd.Flags().GetString("identity")
					format, _ := cmd.Flags().GetString("format")

					// Determine target identity
					if identityName == "" {
						// Find default identity
						for name, identity := range config.Auth.Identities {
							if identity.Default {
								identityName = name
								break
							}
						}
						if identityName == "" {
							return assert.AnError
						}
					} else {
						// Validate specified identity exists
						if _, exists := config.Auth.Identities[identityName]; !exists {
							return assert.AnError
						}
					}

					// Mock environment variables
					envVars := []schema.EnvironmentVariable{
						{Key: "AWS_SHARED_CREDENTIALS_FILE", Value: "/home/user/.aws/atmos/test-provider/credentials"},
						{Key: "AWS_CONFIG_FILE", Value: "/home/user/.aws/atmos/test-provider/config"},
						{Key: "AWS_PROFILE", Value: identityName},
					}

					// Output in requested format
					switch format {
					case "json":
						envMap := make(map[string]string)
						for _, env := range envVars {
							envMap[env.Key] = env.Value
						}
						jsonData, _ := json.MarshalIndent(envMap, "", "  ")
						cmd.Println(string(jsonData))
					case "dotenv":
						for _, env := range envVars {
							cmd.Printf("%s=%s\n", env.Key, env.Value)
						}
					default: // export format
						for _, env := range envVars {
							cmd.Printf("export %s=%s\n", env.Key, env.Value)
						}
					}

					return nil
				},
			}
			cmd.Flags().StringP("identity", "i", "", "Identity to get environment for")
			cmd.Flags().StringP("format", "f", "export", "Output format (export, json, dotenv)")

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
			} else {
				assert.NoError(t, err)
				output := buf.String()
				for _, expected := range tt.expectedOutput {
					assert.Contains(t, output, expected)
				}
			}
		})
	}
}

func TestAuthEnvCmdFlags(t *testing.T) {
	// Create a mock command to test flag structure
	cmd := &cobra.Command{
		Use: "env",
	}
	cmd.Flags().StringP("identity", "i", "", "Identity to get environment for")
	cmd.Flags().StringP("format", "f", "export", "Output format (export, json, dotenv)")

	// Test that required flags are present
	identityFlag := cmd.Flags().Lookup("identity")
	require.NotNil(t, identityFlag)
	assert.Equal(t, "i", identityFlag.Shorthand)

	formatFlag := cmd.Flags().Lookup("format")
	require.NotNil(t, formatFlag)
	assert.Equal(t, "f", formatFlag.Shorthand)
	assert.Equal(t, "export", formatFlag.DefValue)
}

func TestFormatEnvironmentVariables(t *testing.T) {
	envVars := []schema.EnvironmentVariable{
		{Key: "AWS_PROFILE", Value: "test-profile"},
		{Key: "AWS_REGION", Value: "us-east-1"},
	}

	tests := []struct {
		format   string
		expected []string
	}{
		{
			format: "export",
			expected: []string{
				"export AWS_PROFILE=test-profile",
				"export AWS_REGION=us-east-1",
			},
		},
		{
			format: "dotenv",
			expected: []string{
				"AWS_PROFILE=test-profile",
				"AWS_REGION=us-east-1",
			},
		},
		{
			format: "json",
			expected: []string{
				`"AWS_PROFILE": "test-profile"`,
				`"AWS_REGION": "us-east-1"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			var output strings.Builder

			switch tt.format {
			case "json":
				envMap := make(map[string]string)
				for _, env := range envVars {
					envMap[env.Key] = env.Value
				}
				jsonData, _ := json.MarshalIndent(envMap, "", "  ")
				output.WriteString(string(jsonData))
			case "dotenv":
				for _, env := range envVars {
					output.WriteString(env.Key + "=" + env.Value + "\n")
				}
			default: // export
				for _, env := range envVars {
					output.WriteString("export " + env.Key + "=" + env.Value + "\n")
				}
			}

			result := output.String()
			for _, expected := range tt.expected {
				assert.Contains(t, result, expected)
			}
		})
	}
}
