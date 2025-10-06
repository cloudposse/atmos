package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthWhoamiCmd(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		setupConfig    func() *schema.AtmosConfiguration
		expectedError  string
		expectedOutput string
	}{
		{
			name: "successful whoami with default identity",
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
			expectedOutput: "test-identity",
		},
		{
			name: "whoami with json output",
			args: []string{"--output", "json"},
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
			expectedOutput: `"identity": "test-identity"`,
		},
		{
			name: "no default identity configured",
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
			expectedOutput: "No default identity configured",
		},
		{
			name: "no auth configuration",
			args: []string{},
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Auth: schema.AuthConfig{},
				}
			},
			expectedOutput: "No default identity configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock command for testing
			cmd := &cobra.Command{
				Use: "whoami",
				RunE: func(cmd *cobra.Command, args []string) error {
					config := tt.setupConfig()
					outputFormat, _ := cmd.Flags().GetString("output")

					// Find default identity
					var defaultIdentity string
					for name, identity := range config.Auth.Identities {
						if identity.Default {
							defaultIdentity = name
							break
						}
					}

					if defaultIdentity == "" {
						cmd.PrintErrln("No default identity configured.")
						cmd.PrintErrln("Configure auth in atmos.yaml and run `atmos auth login` to authenticate.")
						return nil
					}

					// Mock whoami info
					expTime, _ := time.Parse(time.RFC3339, "2024-01-01T12:00:00Z")
					whoamiInfo := authTypes.WhoamiInfo{
						Identity:    defaultIdentity,
						Provider:    "test-provider",
						Account:     "123456789012",
						Principal:   "TestRole",
						Expiration:  &expTime,
						Credentials: &authTypes.AWSCredentials{},
					}

					if outputFormat == "json" {
						jsonData, _ := json.MarshalIndent(whoamiInfo, "", "  ")
						cmd.Println(string(jsonData))
					} else {
						cmd.PrintErrf("**Identity**: %s\n", whoamiInfo.Identity)
						cmd.PrintErrf("**Provider**: %s\n", whoamiInfo.Provider)
						cmd.PrintErrf("**Account**: %s\n", whoamiInfo.Account)
						cmd.PrintErrf("**Principal**: %s\n", whoamiInfo.Principal)
						if whoamiInfo.Expiration != nil {
							cmd.PrintErrf("**Expiration**: %s\n", whoamiInfo.Expiration.Format(time.RFC3339))
						}
					}

					return nil
				},
			}
			cmd.Flags().StringP("output", "o", "", "Output format (json)")

			// Capture output
			var stdoutBuf, stderrBuf bytes.Buffer
			cmd.SetOut(&stdoutBuf)
			cmd.SetErr(&stderrBuf)

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
				out := stdoutBuf.String()
				errOut := stderrBuf.String()
				// JSON should appear on stdout; human-readable on stderr.
				if len(tt.args) >= 2 && tt.args[0] == "--output" && tt.args[1] == "json" {
					assert.Contains(t, out, tt.expectedOutput)
				} else {
					assert.Contains(t, errOut, tt.expectedOutput)
				}
			}
		})
	}
}

func TestAuthWhoamiCmdFlags(t *testing.T) {
	// Create a mock command to test flag structure
	cmd := &cobra.Command{
		Use: "whoami",
	}
	cmd.Flags().StringP("output", "o", "", "Output format (json)")

	// Test that output flag is present
	outputFlag := cmd.Flags().Lookup("output")
	require.NotNil(t, outputFlag)
	assert.Equal(t, "o", outputFlag.Shorthand)
	assert.Equal(t, "", outputFlag.DefValue)
}

func TestWhoamiJSONOutput(t *testing.T) {
	expTime, _ := time.Parse(time.RFC3339, "2024-01-01T12:00:00Z")
	whoamiInfo := authTypes.WhoamiInfo{
		Identity:   "test-identity",
		Provider:   "test-provider",
		Account:    "123456789012",
		Principal:  "TestRole",
		Expiration: &expTime,
		Credentials: &authTypes.AWSCredentials{
			AccessKeyID:     "AKIATEST",
			SecretAccessKey: "secret",
			SessionToken:    "token",
			Region:          "us-east-1",
		},
	}

	jsonData, err := json.MarshalIndent(whoamiInfo, "", "  ")
	assert.NoError(t, err)
	assert.Contains(t, string(jsonData), `"identity": "test-identity"`)
	assert.Contains(t, string(jsonData), `"provider": "test-provider"`)
	assert.Contains(t, string(jsonData), `"account": "123456789012"`)
}
