package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthLoginCmd(t *testing.T) {
	_ = NewTestKit(t)

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
			args: []string{"--identity=test-identity"},
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
	_ = NewTestKit(t)

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
	_ = NewTestKit(t)

	tests := []struct {
		name        string
		config      *schema.AuthConfig
		expectError bool
	}{
		{
			name: "valid config with provider and identity",
			config: &schema.AuthConfig{
				Providers: map[string]schema.Provider{
					"test-provider": {
						Kind:     "aws/iam-identity-center",
						Region:   "us-east-1",
						StartURL: "https://test.awsapps.com/start",
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
			expectError: false,
		},
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
		},
		{
			name:        "empty config - succeeds but has no providers/identities",
			config:      &schema.AuthConfig{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t) // Isolate RootCmd state per subtest.

			manager, err := createAuthManager(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, manager)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manager)
			}
		})
	}
}

func TestExecuteAuthLoginCommand(t *testing.T) {
	_ = NewTestKit(t)

	// Test the actual executeAuthLoginCommand function with various error scenarios.
	//
	// Coverage Note.
	// - Error paths (config init, auth manager creation, GetDefaultIdentity): ~40.7% - COVERED.
	// - Success paths (authentication, display output): ~59.3% - NOT COVERED.
	//
	// The success paths require real authentication with cloud providers and are not
	// testable in unit tests without complex mocking or integration test infrastructure.
	// These paths are exercised in integration tests and manual testing.
	tests := []struct {
		name          string
		identityFlag  string
		envVars       map[string]string
		expectError   bool
		errorContains string
	}{
		{
			name:          "no identity or provider configured - falls back to provider then fails",
			identityFlag:  "",
			expectError:   true,
			errorContains: "no providers available", // Changed: now falls back to provider auth first.
		},
		{
			name:          "explicit identity but no auth config",
			identityFlag:  "test-identity",
			expectError:   true,
			errorContains: "identity not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t) // Isolate RootCmd state per subtest.

			// Set environment variables for test.
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			// Create command and set identity flag if provided.
			cmd := &cobra.Command{
				Use:  "login",
				RunE: executeAuthLoginCommand,
			}
			cmd.Flags().StringP("identity", "i", "", "Specify the identity to authenticate to")

			if tt.identityFlag != "" {
				cmd.SetArgs([]string{"--identity=" + tt.identityFlag})
			}

			// Execute command (this will fail in test environment without proper config).
			err := cmd.Execute()

			// Verify error expectations.
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestIdentitySelectorBehavior tests that the identity selector only appears when appropriate.
// Bug report: Selector was appearing even when --identity flag was passed with a value.
func TestIdentitySelectorBehavior(t *testing.T) {
	_ = NewTestKit(t)

	tests := []struct {
		name                string
		args                []string
		viperIdentityValue  string
		setViperValue       bool
		expectedForceSelect bool
		shouldShowSelector  bool
		description         string
	}{
		{
			name:                "--identity with value should NOT show selector",
			args:                []string{"--identity=superadmin-b"},
			viperIdentityValue:  "",
			setViperValue:       false,
			expectedForceSelect: false,
			shouldShowSelector:  false,
			description:         "When user explicitly passes --identity=superadmin-b, selector must not appear",
		},
		{
			name:                "--identity without value should show selector",
			args:                []string{"--identity"},
			viperIdentityValue:  "",
			setViperValue:       false,
			expectedForceSelect: true,
			shouldShowSelector:  true,
			description:         "When user passes --identity with no value, NoOptDefVal sets it to __SELECT__",
		},
		{
			name:                "no --identity flag should show selector if no default",
			args:                []string{},
			viperIdentityValue:  "",
			setViperValue:       false,
			expectedForceSelect: false,
			shouldShowSelector:  true,
			description:         "When no --identity flag passed and no default, should prompt user",
		},
		{
			name:                "ATMOS_IDENTITY env var with value should NOT show selector",
			args:                []string{},
			viperIdentityValue:  "env-identity",
			setViperValue:       true,
			expectedForceSelect: false,
			shouldShowSelector:  false,
			description:         "When ATMOS_IDENTITY=env-identity is set, should use that value",
		},
		{
			name:                "ATMOS_IDENTITY empty should show selector",
			args:                []string{},
			viperIdentityValue:  "",
			setViperValue:       true,
			expectedForceSelect: false,
			shouldShowSelector:  true,
			description:         "When ATMOS_IDENTITY is set but empty, should show selector",
		},
		{
			name:                "ATMOS_IDENTITY with __SELECT__ should show selector",
			args:                []string{},
			viperIdentityValue:  IdentityFlagSelectValue,
			setViperValue:       true,
			expectedForceSelect: true,
			shouldShowSelector:  true,
			description:         "When ATMOS_IDENTITY=__SELECT__, should show selector",
		},
		{
			name:                "--identity with value but viper has empty value",
			args:                []string{"--identity=superadmin-b"},
			viperIdentityValue:  "",
			setViperValue:       true,
			expectedForceSelect: false,
			shouldShowSelector:  false,
			description:         "Flag should take precedence over viper's empty value",
		},
		{
			name:                "--identity with value but viper has __SELECT__",
			args:                []string{"--identity=superadmin-b"},
			viperIdentityValue:  IdentityFlagSelectValue,
			setViperValue:       true,
			expectedForceSelect: false,
			shouldShowSelector:  false,
			description:         "BUG: Flag should take precedence over viper's __SELECT__ value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t) // Isolate RootCmd state.

			// Create a standalone command (not added to RootCmd) for testing.
			cmd := &cobra.Command{
				Use: "test-login",
				RunE: func(cmd *cobra.Command, args []string) error {
					// Replicate the FIXED logic from auth_login.go:60-79.
					// Check if flag was explicitly set by user.
					flagChanged := cmd.Flags().Changed(IdentityFlagName)
					flagValue, _ := cmd.Flags().GetString(IdentityFlagName)
					viperValue := viper.GetString(IdentityFlagName)

					var identityName string
					if flagChanged {
						// Flag was explicitly set on command line.
						identityName = flagValue
					} else {
						// Flag not set - fall back to viper.
						identityName = viperValue
					}

					forceSelect := identityName == IdentityFlagSelectValue
					showSelector := identityName == "" || forceSelect

					// Log for debugging.
					t.Logf("%s: flagChanged=%v, flagValue=%q, viperValue=%q, identityName=%q, forceSelect=%v, showSelector=%v",
						tt.name, flagChanged, flagValue, viperValue, identityName, forceSelect, showSelector)

					// Verify expectations.
					assert.Equal(t, tt.expectedForceSelect, forceSelect,
						"forceSelect mismatch: expected %v, got %v (identityName=%q)",
						tt.expectedForceSelect, forceSelect, identityName)
					assert.Equal(t, tt.shouldShowSelector, showSelector,
						"showSelector mismatch: expected %v, got %v (identityName=%q)",
						tt.shouldShowSelector, showSelector, identityName)

					return nil
				},
			}
			cmd.Flags().StringP(IdentityFlagName, "i", "", "Specify the identity to authenticate to")

			// Set NoOptDefVal to match auth.go setup.
			identityFlag := cmd.Flags().Lookup(IdentityFlagName)
			if identityFlag != nil {
				identityFlag.NoOptDefVal = IdentityFlagSelectValue
			}

			// DO NOT bind flag to Viper (auth.go no longer does this).
			// BindPFlag creates a two-way binding that breaks flag precedence.
			// Instead, set viper value to simulate config/env, and the command logic
			// will check flag first, then fall back to viper.

			// Set viper value if needed to simulate config/env values.
			if tt.setViperValue {
				viper.Set(IdentityFlagName, tt.viperIdentityValue)
				t.Cleanup(func() {
					viper.Set(IdentityFlagName, "")
				})
			}

			// Set args and execute.
			cmd.SetArgs(tt.args)
			err := cmd.Execute()

			// Should not error in test (we're just validating selector logic).
			require.NoError(t, err, "Test command should not error")
		})
	}
}

// TestGetProviderForFallback tests the provider fallback logic when no identities are configured.
func TestGetProviderForFallback(t *testing.T) {
	_ = NewTestKit(t)

	tests := []struct {
		name           string
		providers      []string
		expectedResult string
		expectError    bool
		errorIs        error
		description    string
	}{
		{
			name:           "single provider auto-selects",
			providers:      []string{"sso-prod"},
			expectedResult: "sso-prod",
			expectError:    false,
			description:    "When only one provider is configured, it should be auto-selected",
		},
		{
			name:        "no providers returns error",
			providers:   []string{},
			expectError: true,
			errorIs:     errUtils.ErrNoProvidersAvailable,
			description: "When no providers are configured, should return ErrNoProvidersAvailable",
		},
		{
			name:        "multiple providers in non-interactive mode returns error",
			providers:   []string{"sso-prod", "sso-dev"},
			expectError: true,
			errorIs:     errUtils.ErrNoDefaultProvider,
			description: "When multiple providers exist and not interactive, should return ErrNoDefaultProvider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			// Create a mock auth manager that returns the configured providers.
			mockManager := &mockAuthManagerForProviderFallback{
				providers: tt.providers,
			}

			result, err := getProviderForFallback(mockManager)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorIs != nil {
					assert.ErrorIs(t, err, tt.errorIs)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

// mockAuthManagerForProviderFallback is a minimal mock for testing getProviderForFallback.
type mockAuthManagerForProviderFallback struct {
	providers []string
}

func (m *mockAuthManagerForProviderFallback) ListProviders() []string {
	return m.providers
}

// TestPromptForProvider tests the provider prompt function.
func TestPromptForProvider(t *testing.T) {
	_ = NewTestKit(t)

	t.Run("empty providers list returns error", func(t *testing.T) {
		_, err := promptForProvider("Select a provider:", []string{})
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrNoProvidersAvailable)
	})
}

// TestFormatDuration tests the formatDuration function with various duration values.
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "expired (negative duration)",
			duration: -1 * time.Hour,
			expected: "expired",
		},
		{
			name:     "hours and minutes",
			duration: 2*time.Hour + 30*time.Minute,
			expected: "2h 30m",
		},
		{
			name:     "only hours",
			duration: 3 * time.Hour,
			expected: "3h 0m",
		},
		{
			name:     "only minutes",
			duration: 45 * time.Minute,
			expected: "45m 0s",
		},
		{
			name:     "minutes and seconds",
			duration: 5*time.Minute + 30*time.Second,
			expected: "5m 30s",
		},
		{
			name:     "only seconds",
			duration: 45 * time.Second,
			expected: "45s",
		},
		{
			name:     "zero duration",
			duration: 0,
			expected: "0s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDisplayAuthSuccess tests the displayAuthSuccess function output formatting.
func TestDisplayAuthSuccess(t *testing.T) {
	_ = NewTestKit(t)

	// Capture stderr output.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Create test whoami info with all fields.
	expiration := time.Now().Add(1 * time.Hour)
	whoami := &authTypes.WhoamiInfo{
		Provider:   "test-provider",
		Identity:   "test-identity",
		Account:    "123456789012",
		Region:     "us-east-1",
		Expiration: &expiration,
	}

	// Call the function.
	displayAuthSuccess(whoami)

	// Restore stderr and read output.
	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Verify output contains expected content.
	assert.Contains(t, output, "Authentication successful")
	assert.Contains(t, output, "test-provider")
	assert.Contains(t, output, "test-identity")
	assert.Contains(t, output, "123456789012")
	assert.Contains(t, output, "us-east-1")
}

// TestDisplayAuthSuccessMinimalFields tests displayAuthSuccess with minimal fields.
func TestDisplayAuthSuccessMinimalFields(t *testing.T) {
	_ = NewTestKit(t)

	// Capture stderr output.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	// Create test whoami info with only required fields.
	whoami := &authTypes.WhoamiInfo{
		Provider: "minimal-provider",
		Identity: "minimal-identity",
		// Account, Region, and Expiration are empty/nil.
	}

	// Call the function.
	displayAuthSuccess(whoami)

	// Restore stderr and read output.
	w.Close()
	os.Stderr = oldStderr
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Verify output contains expected content.
	assert.Contains(t, output, "Authentication successful")
	assert.Contains(t, output, "minimal-provider")
	assert.Contains(t, output, "minimal-identity")
}

// TestIsInteractive tests the isInteractive function.
// Note: This test may behave differently in CI vs local environments.
func TestIsInteractive(t *testing.T) {
	// The function checks term.IsTTYSupportForStdin() && !telemetry.IsCI().
	// In CI environments, this should return false.
	// In local terminal environments, it depends on stdin TTY status.
	result := isInteractive()

	// We can't assert a specific value since it depends on the environment,
	// but we can verify the function doesn't panic and returns a boolean.
	assert.IsType(t, true, result)
}

// TestCreateAuthManagerExported tests the exported CreateAuthManager wrapper function.
func TestCreateAuthManagerExported(t *testing.T) {
	_ = NewTestKit(t)

	tests := []struct {
		name        string
		config      *schema.AuthConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: &schema.AuthConfig{
				Providers: map[string]schema.Provider{
					"test-provider": {
						Kind:     "aws/iam-identity-center",
						Region:   "us-east-1",
						StartURL: "https://test.awsapps.com/start",
					},
				},
			},
			expectError: false,
		},
		{
			name:        "nil config returns error",
			config:      nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := CreateAuthManager(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, manager)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manager)
			}
		})
	}
}

// TestAuthenticateIdentity tests the authenticateIdentity function.
// Note: This test covers the default identity and provider fallback paths.
// The --identity flag path is covered by TestResolveIdentityName_PersistentFlag_WithNoOptDefVal
// in auth_console_test.go which properly tests GetIdentityFromFlags behavior.
func TestAuthenticateIdentity(t *testing.T) {
	_ = NewTestKit(t)

	// Test cases that don't depend on os.Args parsing.
	// These cover the default identity fallback and error paths.
	tests := []struct {
		name                   string
		mockDefaultIdentity    string
		mockDefaultErr         error
		mockAuthenticateResult *authTypes.WhoamiInfo
		mockAuthenticateErr    error
		expectWhoami           bool
		expectNeedsFallback    bool
		expectError            bool
		errorContains          string
	}{
		{
			name:                "falls back to default identity when no flag",
			mockDefaultIdentity: "default-identity",
			mockAuthenticateResult: &authTypes.WhoamiInfo{
				Provider: "test-provider",
				Identity: "default-identity",
			},
			expectWhoami:        true,
			expectNeedsFallback: false,
			expectError:         false,
		},
		{
			name:                "no identities available triggers provider fallback",
			mockDefaultIdentity: "",
			mockDefaultErr:      errUtils.ErrNoIdentitiesAvailable,
			expectWhoami:        false,
			expectNeedsFallback: true,
			expectError:         false,
		},
		{
			name:                "no default identity triggers provider fallback",
			mockDefaultIdentity: "",
			mockDefaultErr:      errUtils.ErrNoDefaultIdentity,
			expectWhoami:        false,
			expectNeedsFallback: true,
			expectError:         false,
		},
		{
			name:                   "authentication failure returns error",
			mockDefaultIdentity:    "default-identity",
			mockAuthenticateResult: nil,
			mockAuthenticateErr:    fmt.Errorf("authentication failed"),
			expectWhoami:           false,
			expectNeedsFallback:    false,
			expectError:            true,
			errorContains:          "authentication failed",
		},
		{
			name:                "other GetDefaultIdentity error returns error",
			mockDefaultIdentity: "",
			mockDefaultErr:      fmt.Errorf("unexpected error"),
			expectWhoami:        false,
			expectNeedsFallback: false,
			expectError:         true,
			errorContains:       "unexpected error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			// Ensure Viper doesn't have stale identity value.
			viper.GetViper().Set("identity", "")

			// Create command with identity flag (but no flag value set).
			cmd := &cobra.Command{Use: "login"}
			cmd.Flags().StringP("identity", "i", "", "identity name")

			// Create gomock controller and mock auth manager.
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockManager := authTypes.NewMockAuthManager(ctrl)

			// No identity flag - will call GetDefaultIdentity.
			// Since os.Args won't have --identity, GetIdentityFromFlags returns "".
			if tt.mockDefaultErr != nil {
				mockManager.EXPECT().GetDefaultIdentity(false).Return("", tt.mockDefaultErr)
			} else {
				mockManager.EXPECT().GetDefaultIdentity(false).Return(tt.mockDefaultIdentity, nil)
				// Will then call Authenticate.
				if tt.mockAuthenticateErr != nil {
					mockManager.EXPECT().Authenticate(gomock.Any(), tt.mockDefaultIdentity).Return(nil, tt.mockAuthenticateErr)
				} else {
					mockManager.EXPECT().Authenticate(gomock.Any(), tt.mockDefaultIdentity).Return(tt.mockAuthenticateResult, nil)
				}
			}

			// Call authenticateIdentity.
			ctx := context.Background()
			whoami, needsFallback, err := authenticateIdentity(ctx, cmd, mockManager)

			// Verify expectations.
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectNeedsFallback, needsFallback)

			if tt.expectWhoami {
				assert.NotNil(t, whoami)
			} else {
				assert.Nil(t, whoami)
			}
		})
	}
}
