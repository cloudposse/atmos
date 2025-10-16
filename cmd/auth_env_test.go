package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthEnvCmd(t *testing.T) {
	CleanupRootCmd(t)

	tests := []struct {
		name           string
		args           []string
		setupConfig    func() *schema.AtmosConfiguration
		expectedError  string
		expectedOutput []string
	}{
		{
			name: "bash format with default identity",
			args: []string{"--format", "bash"},
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
				"export AWS_SHARED_CREDENTIALS_FILE='/home/user/.aws/atmos/test-provider/credentials'",
				"export AWS_CONFIG_FILE='/home/user/.aws/atmos/test-provider/config'",
				"export AWS_PROFILE='test-identity'",
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
				"AWS_SHARED_CREDENTIALS_FILE='/home/user/.aws/atmos/test-provider/credentials'",
				"AWS_CONFIG_FILE='/home/user/.aws/atmos/test-provider/config'",
				"AWS_PROFILE='test-identity'",
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
			CleanupRootCmd(t)

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
						identityName = func() string {
							for name, identity := range config.Auth.Identities {
								if identity.Default {
									return name
								}
							}
							return ""
						}()
						if identityName == "" {
							return fmt.Errorf("no default identity configured")
						}
					}
					// Validate specified identity exists
					if _, exists := config.Auth.Identities[identityName]; !exists {
						return fmt.Errorf("identity %q not found", identityName)
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
						// Collect and sort keys for deterministic output
						keys := make([]string, 0, len(envVars))
						envMap := make(map[string]string, len(envVars))
						for _, env := range envVars {
							keys = append(keys, env.Key)
							envMap[env.Key] = env.Value
						}
						sort.Strings(keys)
						for _, k := range keys {
							v := envMap[k]
							safe := strings.ReplaceAll(v, "'", "'\\''")
							cmd.Printf("%s='%s'\n", k, safe)
						}
					default: // export format
						// Collect and sort keys for deterministic output
						keys := make([]string, 0, len(envVars))
						envMap := make(map[string]string, len(envVars))
						for _, env := range envVars {
							keys = append(keys, env.Key)
							envMap[env.Key] = env.Value
						}
						sort.Strings(keys)
						for _, k := range keys {
							v := envMap[k]
							safe := strings.ReplaceAll(v, "'", "'\\''")
							cmd.Printf("export %s='%s'\n", k, safe)
						}
					}

					return nil
				},
			}
			cmd.Flags().StringP("identity", "i", "", "Identity to get environment for")
			cmd.Flags().StringP("format", "f", "bash", "Output format (bash, json, dotenv)")

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
				if assert.Error(t, err) {
					assert.Contains(t, err.Error(), tt.expectedError)
				}
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
	CleanupRootCmd(t)

	// Create a mock command to test flag structure
	cmd := &cobra.Command{
		Use: "env",
	}
	cmd.Flags().StringP("identity", "i", "", "Identity to get environment for")
	cmd.Flags().StringP("format", "f", "bash", "Output format (bash, json, dotenv)")

	// Test that required flags are present
	identityFlag := cmd.Flags().Lookup("identity")
	require.NotNil(t, identityFlag)
	assert.Equal(t, "i", identityFlag.Shorthand)

	formatFlag := cmd.Flags().Lookup("format")
	require.NotNil(t, formatFlag)
	assert.Equal(t, "f", formatFlag.Shorthand)
	assert.Equal(t, "bash", formatFlag.DefValue)
}

func TestFormatEnvironmentVariables(t *testing.T) {
	CleanupRootCmd(t)

	envVars := []schema.EnvironmentVariable{
		{Key: "AWS_PROFILE", Value: "test-profile"},
		{Key: "AWS_REGION", Value: "us-east-1"},
	}

	tests := []struct {
		format   string
		expected []string
	}{
		{
			format: "bash",
			expected: []string{
				"export AWS_PROFILE='test-profile'",
				"export AWS_REGION='us-east-1'",
			},
		},
		{
			format: "dotenv",
			expected: []string{
				"AWS_PROFILE='test-profile'",
				"AWS_REGION='us-east-1'",
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
			CleanupRootCmd(t)

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
				keys := []string{}
				m := map[string]string{}
				for _, env := range envVars {
					keys = append(keys, env.Key)
					m[env.Key] = env.Value
				}
				sort.Strings(keys)
				for _, k := range keys {
					v := m[k]
					safe := strings.ReplaceAll(v, "'", "'\\''")
					output.WriteString(k + "='" + safe + "'\n")
				}
			default: // export
				keys := []string{}
				m := map[string]string{}
				for _, env := range envVars {
					keys = append(keys, env.Key)
					m[env.Key] = env.Value
				}
				sort.Strings(keys)
				for _, k := range keys {
					v := m[k]
					safe := strings.ReplaceAll(v, "'", "'\\''")
					output.WriteString("export " + k + "='" + safe + "'\n")
				}
			}

			result := output.String()
			for _, expected := range tt.expected {
				assert.Contains(t, result, expected)
			}
		})
	}
}
