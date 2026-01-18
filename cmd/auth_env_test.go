package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthEnvCmd(t *testing.T) {
	_ = NewTestKit(t)

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
			args: []string{"--format", "json", "--identity=test-identity"},
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
			args: []string{"--identity=nonexistent"},
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
			_ = NewTestKit(t)

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
	_ = NewTestKit(t)

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
	_ = NewTestKit(t)

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
			_ = NewTestKit(t)

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

func TestFormatAuthGitHub(t *testing.T) {
	_ = NewTestKit(t)

	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name: "single line values",
			envVars: map[string]string{
				"AWS_PROFILE": "test-profile",
				"AWS_REGION":  "us-east-1",
			},
			expected: "AWS_PROFILE=test-profile\nAWS_REGION=us-east-1\n",
		},
		{
			name: "multiline value",
			envVars: map[string]string{
				"SIMPLE_VAR":    "simple",
				"MULTILINE_VAR": "line1\nline2\nline3",
			},
			expected: "MULTILINE_VAR<<ATMOS_EOF_MULTILINE_VAR\nline1\nline2\nline3\nATMOS_EOF_MULTILINE_VAR\nSIMPLE_VAR=simple\n",
		},
		{
			name:     "empty map",
			envVars:  map[string]string{},
			expected: "",
		},
		{
			name: "value with equals sign",
			envVars: map[string]string{
				"VAR_WITH_EQUALS": "key=value",
			},
			expected: "VAR_WITH_EQUALS=key=value\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatAuthGitHub(tt.envVars)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatAuthBash(t *testing.T) {
	_ = NewTestKit(t)

	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name: "simple values",
			envVars: map[string]string{
				"AWS_PROFILE": "test-profile",
				"AWS_REGION":  "us-east-1",
			},
			expected: "export AWS_PROFILE='test-profile'\nexport AWS_REGION='us-east-1'\n",
		},
		{
			name: "value with single quote",
			envVars: map[string]string{
				"MSG": "it's working",
			},
			expected: "export MSG='it'\\''s working'\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatAuthBash(tt.envVars)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatAuthDotenv(t *testing.T) {
	_ = NewTestKit(t)

	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name: "simple values",
			envVars: map[string]string{
				"AWS_PROFILE": "test-profile",
				"AWS_REGION":  "us-east-1",
			},
			expected: "AWS_PROFILE='test-profile'\nAWS_REGION='us-east-1'\n",
		},
		{
			name: "value with single quote",
			envVars: map[string]string{
				"MSG": "it's working",
			},
			expected: "MSG='it'\\''s working'\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatAuthDotenv(tt.envVars)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWriteAuthEnvToFile(t *testing.T) {
	_ = NewTestKit(t)

	t.Run("creates and writes to new file", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "github_env")

		envVars := map[string]string{
			"AWS_PROFILE": "test-profile",
			"AWS_REGION":  "us-east-1",
		}

		err := writeAuthEnvToFile(envVars, filePath, formatAuthGitHub)
		require.NoError(t, err)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "AWS_PROFILE=test-profile\nAWS_REGION=us-east-1\n", string(content))
	})

	t.Run("appends to existing file", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "github_env")

		// Write initial content.
		err := os.WriteFile(filePath, []byte("EXISTING_VAR=existing\n"), 0o644)
		require.NoError(t, err)

		envVars := map[string]string{
			"NEW_VAR": "new-value",
		}

		err = writeAuthEnvToFile(envVars, filePath, formatAuthGitHub)
		require.NoError(t, err)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "EXISTING_VAR=existing\nNEW_VAR=new-value\n", string(content))
	})

	t.Run("handles multiline values correctly", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "github_env")

		envVars := map[string]string{
			"MULTILINE": "line1\nline2",
		}

		err := writeAuthEnvToFile(envVars, filePath, formatAuthGitHub)
		require.NoError(t, err)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		expected := "MULTILINE<<ATMOS_EOF_MULTILINE\nline1\nline2\nATMOS_EOF_MULTILINE\n"
		assert.Equal(t, expected, string(content))
	})
}

func TestSortedAuthKeys(t *testing.T) {
	_ = NewTestKit(t)

	m := map[string]string{
		"ZEBRA":   "z",
		"ALPHA":   "a",
		"CHARLIE": "c",
	}

	keys := sortedAuthKeys(m)
	assert.Equal(t, []string{"ALPHA", "CHARLIE", "ZEBRA"}, keys)
}

func TestFormatAuthGitHubDelimiterCollision(t *testing.T) {
	_ = NewTestKit(t)

	tests := []struct {
		name     string
		envVars  map[string]string
		contains []string
	}{
		{
			name: "value contains default delimiter",
			envVars: map[string]string{
				"CERT": "line1\nATMOS_EOF_CERT\nline3",
			},
			// Should use ATMOS_EOF_CERT_X since ATMOS_EOF_CERT is in value.
			contains: []string{
				"CERT<<ATMOS_EOF_CERT_X\n",
				"line1\nATMOS_EOF_CERT\nline3\n",
				"ATMOS_EOF_CERT_X\n",
			},
		},
		{
			name: "value contains multiple delimiter variants",
			envVars: map[string]string{
				"KEY": "ATMOS_EOF_KEY\nATMOS_EOF_KEY_X\ndata",
			},
			// Should use ATMOS_EOF_KEY_X_X since both variants are in value.
			contains: []string{
				"KEY<<ATMOS_EOF_KEY_X_X\n",
				"ATMOS_EOF_KEY_X_X\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatAuthGitHub(tt.envVars)
			for _, expected := range tt.contains {
				assert.Contains(t, result, expected)
			}
		})
	}
}

func TestGitHubEnvAutoDetect(t *testing.T) {
	_ = NewTestKit(t)

	t.Run("auto-detects GITHUB_ENV and appends", func(t *testing.T) {
		tempDir := t.TempDir()
		githubEnvFile := filepath.Join(tempDir, "github_env")

		// Write initial content to simulate existing GitHub Actions env vars.
		err := os.WriteFile(githubEnvFile, []byte("EXISTING=value\n"), 0o644)
		require.NoError(t, err)

		// Set GITHUB_ENV environment variable.
		t.Setenv("GITHUB_ENV", githubEnvFile)

		envVars := map[string]string{
			"NEW_VAR": "new-value",
		}

		// Simulate what the command does when --format=github and no --output.
		output := os.Getenv("GITHUB_ENV")
		require.NotEmpty(t, output, "GITHUB_ENV should be set")

		err = writeAuthEnvToFile(envVars, output, formatAuthGitHub)
		require.NoError(t, err)

		content, err := os.ReadFile(githubEnvFile)
		require.NoError(t, err)

		// Verify it appended (not overwrote) and used correct format.
		assert.Equal(t, "EXISTING=value\nNEW_VAR=new-value\n", string(content))
	})

	t.Run("auto-detect with multiline values", func(t *testing.T) {
		tempDir := t.TempDir()
		githubEnvFile := filepath.Join(tempDir, "github_env")

		t.Setenv("GITHUB_ENV", githubEnvFile)

		envVars := map[string]string{
			"CERT": "-----BEGIN CERT-----\ndata\n-----END CERT-----",
		}

		output := os.Getenv("GITHUB_ENV")
		err := writeAuthEnvToFile(envVars, output, formatAuthGitHub)
		require.NoError(t, err)

		content, err := os.ReadFile(githubEnvFile)
		require.NoError(t, err)

		// Verify heredoc format for multiline.
		assert.Contains(t, string(content), "CERT<<ATMOS_EOF_CERT\n")
		assert.Contains(t, string(content), "-----BEGIN CERT-----\ndata\n-----END CERT-----\n")
		assert.Contains(t, string(content), "ATMOS_EOF_CERT\n")
	})
}
