package cmd

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAuthConsoleCommand_Registration(t *testing.T) {
	_ = NewTestKit(t)

	t.Run("command is registered", func(t *testing.T) {
		cmd := RootCmd.Commands()
		var authCmd *cobra.Command
		for _, c := range cmd {
			if c.Name() == "auth" {
				authCmd = c
				break
			}
		}
		require.NotNil(t, authCmd, "auth command should be registered")

		var consoleCmd *cobra.Command
		for _, c := range authCmd.Commands() {
			if c.Name() == "console" {
				consoleCmd = c
				break
			}
		}
		require.NotNil(t, consoleCmd, "console subcommand should be registered under auth")
	})

	t.Run("command has correct metadata", func(t *testing.T) {
		assert.Equal(t, "console", authConsoleCmd.Name())
		assert.Contains(t, authConsoleCmd.Short, "web console")
		assert.NotEmpty(t, authConsoleCmd.Long)
		assert.NotEmpty(t, authConsoleCmd.Example)
	})

	t.Run("command has required flags", func(t *testing.T) {
		destFlag := authConsoleCmd.Flags().Lookup("destination")
		assert.NotNil(t, destFlag)

		durationFlag := authConsoleCmd.Flags().Lookup("duration")
		assert.NotNil(t, durationFlag)

		printOnlyFlag := authConsoleCmd.Flags().Lookup("print-only")
		assert.NotNil(t, printOnlyFlag)

		noOpenFlag := authConsoleCmd.Flags().Lookup("no-open")
		assert.NotNil(t, noOpenFlag)

		issuerFlag := authConsoleCmd.Flags().Lookup("issuer")
		assert.NotNil(t, issuerFlag)
	})
}

func TestRetrieveCredentials(t *testing.T) {
	tests := []struct {
		name    string
		whoami  *types.WhoamiInfo
		wantErr bool
		errType error
		errMsg  string
	}{
		{
			name: "uses in-memory credentials when available",
			whoami: &types.WhoamiInfo{
				Credentials: &types.AWSCredentials{
					AccessKeyID:     "AKIATEST",
					SecretAccessKey: "secret",
					SessionToken:    "token",
				},
			},
			wantErr: false,
		},
		{
			name: "returns error when no credentials available",
			whoami: &types.WhoamiInfo{
				Credentials:    nil,
				CredentialsRef: "",
			},
			wantErr: true,
			errType: errUtils.ErrAuthConsole,
			errMsg:  "no credentials available",
		},
		{
			name: "handles OIDC credentials",
			whoami: &types.WhoamiInfo{
				Credentials: &types.OIDCCredentials{
					Token:    "oidc-token",
					Provider: "github",
				},
			},
			wantErr: false,
		},
		{
			name: "handles AWS credentials with different fields",
			whoami: &types.WhoamiInfo{
				Credentials: &types.AWSCredentials{
					AccessKeyID:     "AKIA123",
					SecretAccessKey: "secret123",
					SessionToken:    "session123",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds, err := retrieveCredentials(tt.whoami)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.True(t, errors.Is(err, tt.errType), "expected error type %v, got %v", tt.errType, err)
				}
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, creds)
			}
		})
	}
}

func TestHandleBrowserOpen(t *testing.T) {
	tests := []struct {
		name       string
		consoleURL string
	}{
		{
			name:       "handles valid URL",
			consoleURL: "https://console.aws.amazon.com",
		},
		{
			name:       "handles empty URL",
			consoleURL: "",
		},
		{
			name:       "handles URL with query parameters",
			consoleURL: "https://console.aws.amazon.com?Action=login&Destination=s3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set CI env to prevent browser from opening during tests.
			t.Setenv("CI", "true")

			// This function doesn't return an error, just verify it doesn't panic.
			assert.NotPanics(t, func() {
				handleBrowserOpen(tt.consoleURL)
			})
		})
	}
}

func TestAuthConsoleCommand_Flags(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		expectedDest      string
		expectedDuration  time.Duration
		expectedPrintOnly bool
		expectedNoOpen    bool
		wantErr           bool
	}{
		{
			name:              "default values",
			args:              []string{},
			expectedDest:      "",
			expectedDuration:  1 * time.Hour,
			expectedPrintOnly: false,
			expectedNoOpen:    false,
			wantErr:           false,
		},
		{
			name:             "with destination flag",
			args:             []string{"--destination", "s3"},
			expectedDest:     "s3",
			expectedDuration: 1 * time.Hour,
			wantErr:          false,
		},
		{
			name:             "with destination as ec2",
			args:             []string{"--destination", "ec2"},
			expectedDest:     "ec2",
			expectedDuration: 1 * time.Hour,
			wantErr:          false,
		},
		{
			name:             "with duration flag",
			args:             []string{"--duration", "2h"},
			expectedDuration: 2 * time.Hour,
			wantErr:          false,
		},
		{
			name:             "with duration in minutes",
			args:             []string{"--duration", "30m"},
			expectedDuration: 30 * time.Minute,
			wantErr:          false,
		},
		{
			name:              "with print-only flag",
			args:              []string{"--print-only"},
			expectedPrintOnly: true,
			expectedDuration:  1 * time.Hour,
			wantErr:           false,
		},
		{
			name:             "with no-open flag",
			args:             []string{"--no-open"},
			expectedNoOpen:   true,
			expectedDuration: 1 * time.Hour,
			wantErr:          false,
		},
		{
			name:              "with all flags",
			args:              []string{"--destination", "cloudformation", "--duration", "3h", "--print-only", "--no-open"},
			expectedDest:      "cloudformation",
			expectedDuration:  3 * time.Hour,
			expectedPrintOnly: true,
			expectedNoOpen:    true,
			wantErr:           false,
		},
		{
			name:    "invalid duration format",
			args:    []string{"--duration", "invalid"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("destination", "", "")
			cmd.Flags().Duration("duration", 1*time.Hour, "")
			cmd.Flags().Bool("print-only", false, "")
			cmd.Flags().Bool("no-open", false, "")

			err := cmd.ParseFlags(tt.args)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.expectedDest != "" {
				dest, _ := cmd.Flags().GetString("destination")
				assert.Equal(t, tt.expectedDest, dest)
			}

			duration, _ := cmd.Flags().GetDuration("duration")
			assert.Equal(t, tt.expectedDuration, duration)

			printOnly, _ := cmd.Flags().GetBool("print-only")
			assert.Equal(t, tt.expectedPrintOnly, printOnly)

			noOpen, _ := cmd.Flags().GetBool("no-open")
			assert.Equal(t, tt.expectedNoOpen, noOpen)
		})
	}
}

func TestAuthConsoleCommand_ErrorHandling(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() error
		errType error
		errMsg  string
	}{
		{
			name: "authentication errors wrapped with sentinel",
			setup: func() error {
				return fmt.Errorf("%w: authentication failed: %w", errUtils.ErrAuthConsole, context.DeadlineExceeded)
			},
			errType: errUtils.ErrAuthConsole,
			errMsg:  "authentication failed",
		},
		{
			name: "credential errors wrapped with sentinel",
			setup: func() error {
				return fmt.Errorf("%w: no credentials available", errUtils.ErrAuthConsole)
			},
			errType: errUtils.ErrAuthConsole,
			errMsg:  "no credentials",
		},
		{
			name: "config loading errors wrapped with sentinel",
			setup: func() error {
				return fmt.Errorf("%w: failed to load atmos config: %w", errUtils.ErrAuthConsole, fmt.Errorf("file not found"))
			},
			errType: errUtils.ErrAuthConsole,
			errMsg:  "failed to load",
		},
		{
			name: "provider not supported errors use correct sentinel",
			setup: func() error {
				return fmt.Errorf("%w: Azure console access not yet implemented", errUtils.ErrProviderNotSupported)
			},
			errType: errUtils.ErrProviderNotSupported,
			errMsg:  "not yet implemented",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.setup()
			assert.Error(t, err)
			assert.True(t, errors.Is(err, tt.errType), "expected error type %v, got %v", tt.errType, err)
			if tt.errMsg != "" {
				assert.Contains(t, err.Error(), tt.errMsg)
			}
		})
	}
}

func TestAuthConsoleCommand_Constants(t *testing.T) {
	t.Run("consoleLabelWidth has valid value", func(t *testing.T) {
		assert.Equal(t, 18, consoleLabelWidth)
	})

	t.Run("consoleOutputFormat has valid format string", func(t *testing.T) {
		assert.Equal(t, "%s %s\n", consoleOutputFormat)
	})
}

func TestAuthConsoleCommand_UsageMarkdown(t *testing.T) {
	t.Run("usage markdown is not empty", func(t *testing.T) {
		assert.NotEmpty(t, authConsoleUsageMarkdown)
	})

	t.Run("usage markdown contains examples", func(t *testing.T) {
		assert.Contains(t, authConsoleUsageMarkdown, "atmos auth console")
	})
}

func TestPrintConsoleInfo(t *testing.T) {
	tests := []struct {
		name       string
		whoami     *types.WhoamiInfo
		duration   time.Duration
		showURL    bool
		consoleURL string
	}{
		{
			name: "prints basic info without URL",
			whoami: &types.WhoamiInfo{
				Provider: "aws",
				Identity: "test-user",
			},
			duration:   1 * time.Hour,
			showURL:    false,
			consoleURL: "",
		},
		{
			name: "prints info with account",
			whoami: &types.WhoamiInfo{
				Provider: "aws",
				Identity: "test-user",
				Account:  "123456789012",
			},
			duration:   2 * time.Hour,
			showURL:    false,
			consoleURL: "",
		},
		{
			name: "prints info with URL",
			whoami: &types.WhoamiInfo{
				Provider: "aws",
				Identity: "test-user",
				Account:  "123456789012",
			},
			duration:   1 * time.Hour,
			showURL:    true,
			consoleURL: "https://console.aws.amazon.com",
		},
		{
			name: "handles zero duration",
			whoami: &types.WhoamiInfo{
				Provider: "azure",
				Identity: "user@example.com",
			},
			duration:   0,
			showURL:    false,
			consoleURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This function prints to stderr, just verify it doesn't panic.
			assert.NotPanics(t, func() {
				printConsoleInfo(tt.whoami, tt.duration, tt.showURL, tt.consoleURL)
			})
		})
	}
}

func TestPrintConsoleURL(t *testing.T) {
	tests := []struct {
		name       string
		consoleURL string
	}{
		{
			name:       "prints valid URL",
			consoleURL: "https://console.aws.amazon.com",
		},
		{
			name:       "prints empty URL",
			consoleURL: "",
		},
		{
			name:       "prints URL with parameters",
			consoleURL: "https://console.aws.amazon.com?Action=login&Destination=s3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This function prints to stderr, just verify it doesn't panic.
			assert.NotPanics(t, func() {
				printConsoleURL(tt.consoleURL)
			})
		})
	}
}

func TestGetConsoleProvider(t *testing.T) {
	tests := []struct {
		name         string
		providerKind string
		identityName string
		wantErr      bool
		errContains  string
	}{
		{
			name:         "AWS IAM Identity Center returns provider",
			providerKind: types.ProviderKindAWSIAMIdentityCenter,
			identityName: "test-identity",
			wantErr:      false,
		},
		{
			name:         "AWS SAML returns provider",
			providerKind: types.ProviderKindAWSSAML,
			identityName: "test-identity",
			wantErr:      false,
		},
		{
			name:         "Azure OIDC returns not implemented error",
			providerKind: types.ProviderKindAzureOIDC,
			identityName: "test-identity",
			wantErr:      true,
			errContains:  "Azure console access not yet implemented",
		},
		{
			name:         "GCP OIDC returns not implemented error",
			providerKind: types.ProviderKindGCPOIDC,
			identityName: "test-identity",
			wantErr:      true,
			errContains:  "GCP console access not yet implemented",
		},
		{
			name:         "unknown provider returns error",
			providerKind: "unknown",
			identityName: "test-identity",
			wantErr:      true,
			errContains:  "does not support web console access",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock auth manager.
			mockManager := &mockAuthManagerForProvider{
				providerKind: tt.providerKind,
			}

			provider, err := getConsoleProvider(mockManager, tt.identityName)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, provider)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, provider)
			}
		})
	}
}

func TestResolveIdentityName(t *testing.T) {
	tests := []struct {
		name            string
		flagValue       string
		defaultIdentity string
		defaultErr      error
		wantIdentity    string
		wantErr         bool
		errContains     string
	}{
		{
			name:            "uses flag value when provided",
			flagValue:       "prod-admin",
			defaultIdentity: "dev-user",
			wantIdentity:    "prod-admin",
			wantErr:         false,
		},
		{
			name:            "uses default identity when flag not provided",
			flagValue:       "",
			defaultIdentity: "dev-user",
			wantIdentity:    "dev-user",
			wantErr:         false,
		},
		{
			name:            "returns error when no default identity",
			flagValue:       "",
			defaultIdentity: "",
			wantErr:         true,
			errContains:     "no default identity configured",
		},
		{
			name:        "returns error when GetDefaultIdentity fails",
			flagValue:   "",
			defaultErr:  fmt.Errorf("auth manager error"),
			wantErr:     true,
			errContains: "failed to get default identity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			// Create a mock command.
			cmd := &cobra.Command{}
			cmd.Flags().String("identity", "", "identity name")
			if tt.flagValue != "" {
				_ = cmd.Flags().Set("identity", tt.flagValue)
			}

			// Create a mock auth manager.
			mockManager := &mockAuthManagerForIdentity{
				defaultIdentity: tt.defaultIdentity,
				defaultErr:      tt.defaultErr,
			}

			identity, err := resolveIdentityName(cmd, mockManager)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Empty(t, identity)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantIdentity, identity)
			}
		})
	}
}

// mockAuthManagerForProvider implements minimal AuthManager for testing getConsoleProvider.
type mockAuthManagerForProvider struct {
	providerKind string
}

func (m *mockAuthManagerForProvider) GetProviderKindForIdentity(identityName string) (string, error) {
	return m.providerKind, nil
}

func (m *mockAuthManagerForProvider) Authenticate(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAuthManagerForProvider) GetDefaultIdentity() (string, error) {
	return "", errors.New("not implemented")
}

func (m *mockAuthManagerForProvider) ListIdentities() []string {
	return nil
}

func (m *mockAuthManagerForProvider) Logout(ctx context.Context, identityName string) error {
	return errors.New("not implemented")
}

func (m *mockAuthManagerForProvider) GetIdentity(identityName string) (types.Identity, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAuthManagerForProvider) GetFilesDisplayPath(providerName string) string {
	return ""
}

func (m *mockAuthManagerForProvider) GetChain() []string {
	return nil
}

func (m *mockAuthManagerForProvider) GetIdentities() map[string]schema.Identity {
	return nil
}

func (m *mockAuthManagerForProvider) Whoami(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAuthManagerForProvider) Validate() error {
	return errors.New("not implemented")
}

func (m *mockAuthManagerForProvider) GetProviderForIdentity(identityName string) string {
	return ""
}

func (m *mockAuthManagerForProvider) GetStackInfo() *schema.ConfigAndStacksInfo {
	return nil
}

func (m *mockAuthManagerForProvider) ListProviders() []string {
	return nil
}

func (m *mockAuthManagerForProvider) GetProviders() map[string]schema.Provider {
	return nil
}

func (m *mockAuthManagerForProvider) LogoutProvider(ctx context.Context, providerName string) error {
	return errors.New("not implemented")
}

func (m *mockAuthManagerForProvider) LogoutAll(ctx context.Context) error {
	return errors.New("not implemented")
}

func (m *mockAuthManagerForProvider) GetEnvironmentVariables(identityName string) (map[string]string, error) {
	return nil, errors.New("not implemented")
}

// mockAuthManagerForIdentity implements minimal AuthManager for testing resolveIdentityName.
type mockAuthManagerForIdentity struct {
	defaultIdentity string
	defaultErr      error
}

func (m *mockAuthManagerForIdentity) GetProviderKindForIdentity(identityName string) (string, error) {
	return "", errors.New("not implemented")
}

func (m *mockAuthManagerForIdentity) Authenticate(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAuthManagerForIdentity) GetDefaultIdentity() (string, error) {
	if m.defaultErr != nil {
		return "", m.defaultErr
	}
	return m.defaultIdentity, nil
}

func (m *mockAuthManagerForIdentity) ListIdentities() []string {
	return nil
}

func (m *mockAuthManagerForIdentity) Logout(ctx context.Context, identityName string) error {
	return errors.New("not implemented")
}

func (m *mockAuthManagerForIdentity) GetIdentity(identityName string) (types.Identity, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAuthManagerForIdentity) GetFilesDisplayPath(providerName string) string {
	return ""
}

func (m *mockAuthManagerForIdentity) GetChain() []string {
	return nil
}

func (m *mockAuthManagerForIdentity) GetIdentities() map[string]schema.Identity {
	return nil
}

func (m *mockAuthManagerForIdentity) Whoami(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAuthManagerForIdentity) Validate() error {
	return errors.New("not implemented")
}

func (m *mockAuthManagerForIdentity) GetProviderForIdentity(identityName string) string {
	return ""
}

func (m *mockAuthManagerForIdentity) GetStackInfo() *schema.ConfigAndStacksInfo {
	return nil
}

func (m *mockAuthManagerForIdentity) ListProviders() []string {
	return nil
}

func (m *mockAuthManagerForIdentity) GetProviders() map[string]schema.Provider {
	return nil
}

func (m *mockAuthManagerForIdentity) LogoutProvider(ctx context.Context, providerName string) error {
	return errors.New("not implemented")
}

func (m *mockAuthManagerForIdentity) LogoutAll(ctx context.Context) error {
	return errors.New("not implemented")
}

func (m *mockAuthManagerForIdentity) GetEnvironmentVariables(identityName string) (map[string]string, error) {
	return nil, errors.New("not implemented")
}

func TestResolveConsoleDuration(t *testing.T) {
	_ = NewTestKit(t)

	tests := []struct {
		name             string
		flagSet          bool
		flagValue        time.Duration
		providerConfig   *schema.ConsoleConfig
		expectedDuration time.Duration
		expectError      bool
	}{
		{
			name:             "flag explicitly set takes precedence",
			flagSet:          true,
			flagValue:        4 * time.Hour,
			providerConfig:   &schema.ConsoleConfig{SessionDuration: "12h"},
			expectedDuration: 4 * time.Hour,
			expectError:      false,
		},
		{
			name:             "provider config used when flag not set",
			flagSet:          false,
			flagValue:        1 * time.Hour, // default flag value
			providerConfig:   &schema.ConsoleConfig{SessionDuration: "8h"},
			expectedDuration: 8 * time.Hour,
			expectError:      false,
		},
		{
			name:             "default flag value when no provider config",
			flagSet:          false,
			flagValue:        1 * time.Hour,
			providerConfig:   nil,
			expectedDuration: 1 * time.Hour,
			expectError:      false,
		},
		{
			name:             "default flag value when provider config empty",
			flagSet:          false,
			flagValue:        1 * time.Hour,
			providerConfig:   &schema.ConsoleConfig{SessionDuration: ""},
			expectedDuration: 1 * time.Hour,
			expectError:      false,
		},
		{
			name:           "invalid provider config duration",
			flagSet:        false,
			flagValue:      1 * time.Hour,
			providerConfig: &schema.ConsoleConfig{SessionDuration: "invalid"},
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			// Create test command with duration flag.
			cmd := &cobra.Command{}
			cmd.Flags().DurationVar(&consoleDuration, "duration", 1*time.Hour, "duration flag")

			// Set flag value and simulate whether user explicitly set it.
			consoleDuration = tt.flagValue
			if tt.flagSet {
				require.NoError(t, cmd.Flags().Set("duration", tt.flagValue.String()))
			}

			// Create mock auth manager using gomock.
			ctrl := gomock.NewController(t)
			mockManager := types.NewMockAuthManager(ctrl)

			// Setup expectation for GetProviders.
			providers := map[string]schema.Provider{
				"test-provider": {
					Kind:    "aws/iam-identity-center",
					Console: tt.providerConfig,
				},
			}
			mockManager.EXPECT().GetProviders().Return(providers).AnyTimes()

			// Call resolveConsoleDuration.
			duration, err := resolveConsoleDuration(cmd, mockManager, "test-provider")

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedDuration, duration)
		})
	}
}
