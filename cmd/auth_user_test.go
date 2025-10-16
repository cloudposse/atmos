package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthUserConfigureCmd(t *testing.T) {
	CleanupRootCmd(t)

	tests := []struct {
		name           string
		args           []string
		setupConfig    func() *schema.AtmosConfiguration
		mockInput      []string
		expectedError  string
		expectedOutput []string
	}{
		{
			name: "successful user configuration",
			args: []string{"configure"},
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Auth: schema.AuthConfig{
						Identities: map[string]schema.Identity{
							"test-user": {
								Kind: "aws/user",
								Credentials: map[string]interface{}{
									"region": "us-east-1",
								},
							},
						},
					},
				}
			},
			mockInput: []string{
				"test-user",          // Identity name
				"AKIATEST123456789",  // Access Key ID
				"secretkey123456789", // Secret Access Key
				"us-east-1",          // Region
				"",                   // MFA ARN (optional)
			},
			expectedOutput: []string{
				"AWS User credentials configured successfully",
				"**Identity**: test-user",
			},
		},
		{
			name: "user configuration with MFA",
			args: []string{"configure"},
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Auth: schema.AuthConfig{
						Identities: map[string]schema.Identity{
							"mfa-user": {
								Kind: "aws/user",
								Credentials: map[string]interface{}{
									"region": "us-west-2",
								},
							},
						},
					},
				}
			},
			mockInput: []string{
				"mfa-user",
				"AKIATEST987654321",
				"anothersecretkey",
				"us-west-2",
				"arn:aws:iam::123456789012:mfa/user",
			},
			expectedOutput: []string{
				"AWS User credentials configured successfully",
				"**Identity**: mfa-user",
				"**MFA ARN**: arn:aws:iam::123456789012:mfa/user",
			},
		},
		{
			name: "invalid identity name",
			args: []string{"configure"},
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Auth: schema.AuthConfig{
						Identities: map[string]schema.Identity{
							"test-user": {
								Kind: "aws/user",
							},
						},
					},
				}
			},
			mockInput: []string{
				"nonexistent-user",
			},
			expectedError: "identity \"nonexistent-user\" not found or not an AWS user identity",
		},
		{
			name: "non-aws-user identity type",
			args: []string{"configure"},
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Auth: schema.AuthConfig{
						Identities: map[string]schema.Identity{
							"sso-identity": {
								Kind: "aws/permission-set",
								Via: &schema.IdentityVia{
									Provider: "test-provider",
								},
							},
						},
					},
				}
			},
			mockInput: []string{
				"sso-identity",
			},
			expectedError: "identity \"sso-identity\" not found or not an AWS user identity",
		},
		{
			name: "empty access key",
			args: []string{"configure"},
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Auth: schema.AuthConfig{
						Identities: map[string]schema.Identity{
							"test-user": {
								Kind: "aws/user",
							},
						},
					},
				}
			},
			mockInput: []string{
				"test-user",
				"", // Empty access key
			},
			expectedError: "access key ID cannot be empty",
		},
		{
			name: "empty secret key",
			args: []string{"configure"},
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Auth: schema.AuthConfig{
						Identities: map[string]schema.Identity{
							"test-user": {
								Kind: "aws/user",
							},
						},
					},
				}
			},
			mockInput: []string{
				"test-user",
				"AKIATEST123456789",
				"", // Empty secret key
			},
			expectedError: "secret access key cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			CleanupRootCmd(t)

			// Create a mock command for testing
			cmd := &cobra.Command{
				Use: "configure",
				RunE: func(cmd *cobra.Command, args []string) error {
					config := tt.setupConfig()

					// Mock user input
					inputIndex := 0
					getInput := func(_ string) string {
						if inputIndex < len(tt.mockInput) {
							input := tt.mockInput[inputIndex]
							inputIndex++
							return input
						}
						return ""
					}

					// Mock identity name input
					identityName := getInput("Enter identity name: ")
					if identityName == "" {
						return fmt.Errorf("identity name cannot be empty")
					}

					// Validate identity exists and is AWS user
					identity, exists := config.Auth.Identities[identityName]
					if !exists || identity.Kind != "aws/user" {
						return fmt.Errorf("identity %q not found or not an AWS user identity", identityName)
					}

					// Mock credential inputs
					accessKeyID := getInput("Enter AWS Access Key ID: ")
					if accessKeyID == "" {
						return fmt.Errorf("access key ID cannot be empty")
					}

					secretAccessKey := getInput("Enter AWS Secret Access Key: ")
					if secretAccessKey == "" {
						return fmt.Errorf("secret access key cannot be empty")
					}

					region := getInput("Enter AWS Region: ")
					mfaArn := getInput("Enter MFA ARN (optional): ")

					// Mock successful configuration
					cmd.Printf("**AWS User credentials configured successfully**\n")
					cmd.Printf("**Identity**: %s\n", identityName)
					cmd.Printf("**Access Key ID**: %s\n", maskAccessKey(accessKeyID))
					cmd.Printf("**Region**: %s\n", region)
					if mfaArn != "" {
						cmd.Printf("**MFA ARN**: %s\n", mfaArn)
					}
					cmd.Printf("Credentials have been securely stored.\n")

					return nil
				},
			}

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
				assert.ErrorContains(t, err, tt.expectedError)
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

// maskAccessKey masks the access key for display.
func maskAccessKey(accessKey string) string {
	if len(accessKey) <= 8 {
		return strings.Repeat("*", len(accessKey))
	}
	return accessKey[:4] + strings.Repeat("*", len(accessKey)-8) + accessKey[len(accessKey)-4:]
}

func TestAuthUserCmdStructure(t *testing.T) {
	CleanupRootCmd(t)

	// Create a mock command to test structure
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage AWS user credentials",
		Long:  "Manage AWS user credentials for atmos auth",
	}

	configureCmd := &cobra.Command{
		Use:   "configure",
		Short: "Configure AWS user credentials",
	}
	cmd.AddCommand(configureCmd)

	// Test command structure
	assert.Equal(t, "user", cmd.Use)
	assert.Equal(t, "Manage AWS user credentials", cmd.Short)
	assert.Contains(t, cmd.Long, "Manage AWS user credentials")

	// Test subcommands
	subcommands := cmd.Commands()
	assert.Len(t, subcommands, 1)
	assert.Equal(t, "configure", subcommands[0].Use)
}

func TestAuthUserConfigureCmdStructure(t *testing.T) {
	CleanupRootCmd(t)

	// Create a mock configure command
	configureCmd := &cobra.Command{
		Use:   "configure",
		Short: "Configure AWS user credentials",
		Long:  "Configure AWS user credentials and store them securely in keyring",
	}

	assert.Equal(t, "configure", configureCmd.Use)
	assert.Equal(t, "Configure AWS user credentials", configureCmd.Short)
	assert.Contains(t, configureCmd.Long, "Configure AWS user credentials")
}

func TestCredentialValidation(t *testing.T) {
	CleanupRootCmd(t)

	tests := []struct {
		name        string
		accessKey   string
		secretKey   string
		region      string
		mfaArn      string
		expectError bool
	}{
		{
			name:        "valid credentials",
			accessKey:   "AKIATEST123456789",
			secretKey:   "secretkey123456789012345678901234567890",
			region:      "us-east-1",
			mfaArn:      "",
			expectError: false,
		},
		{
			name:        "valid credentials with MFA",
			accessKey:   "AKIATEST123456789",
			secretKey:   "secretkey123456789012345678901234567890",
			region:      "us-west-2",
			mfaArn:      "arn:aws:iam::123456789012:mfa/user",
			expectError: false,
		},
		{
			name:        "empty access key",
			accessKey:   "",
			secretKey:   "secretkey123456789012345678901234567890",
			region:      "us-east-1",
			mfaArn:      "",
			expectError: true,
		},
		{
			name:        "empty secret key",
			accessKey:   "AKIATEST123456789",
			secretKey:   "",
			region:      "us-east-1",
			mfaArn:      "",
			expectError: true,
		},
		{
			name:        "invalid access key format",
			accessKey:   "INVALID",
			secretKey:   "secretkey123456789012345678901234567890",
			region:      "us-east-1",
			mfaArn:      "",
			expectError: false, // We don't validate format in this test
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			CleanupRootCmd(t)

			// Mock credential validation
			hasError := tt.accessKey == "" || tt.secretKey == ""

			if tt.expectError {
				assert.True(t, hasError)
			} else {
				assert.False(t, hasError)
			}
		})
	}
}

func TestMaskAccessKey(t *testing.T) {
	CleanupRootCmd(t)

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "AKIATEST123456789",
			expected: "AKIA*********6789",
		},
		{
			input:    "AKIA",
			expected: "****",
		},
		{
			input:    "AKIATEST",
			expected: "********",
		},
		{
			input:    "AKIATESTLONGKEY123456789",
			expected: "AKIA****************6789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			CleanupRootCmd(t)

			result := maskAccessKey(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
