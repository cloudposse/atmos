package auth

import (
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveIdentityName_Console(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name           string
		setupMock      func(*authTypes.MockAuthManager)
		identityFlag   string
		viperIdentity  string
		expectedResult string
		expectedError  error
	}{
		{
			name: "uses default identity when no flag set",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetDefaultIdentity(false).Return("default-identity", nil)
			},
			identityFlag:   "",
			viperIdentity:  "",
			expectedResult: "default-identity",
			expectedError:  nil,
		},
		{
			name: "uses flag value when set",
			setupMock: func(m *authTypes.MockAuthManager) {
				// No mock calls expected when flag is set.
			},
			identityFlag:   "flag-identity",
			viperIdentity:  "",
			expectedResult: "flag-identity",
			expectedError:  nil,
		},
		{
			name: "force select triggers interactive selection",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetDefaultIdentity(true).Return("selected-identity", nil)
			},
			identityFlag:   IdentityFlagSelectValue,
			viperIdentity:  "",
			expectedResult: "selected-identity",
			expectedError:  nil,
		},
		{
			name: "no default identity returns error",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetDefaultIdentity(false).Return("", errUtils.ErrNoDefaultIdentity)
			},
			identityFlag:   "",
			viperIdentity:  "",
			expectedResult: "",
			expectedError:  errUtils.ErrAuthConsole,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAuthManager := authTypes.NewMockAuthManager(ctrl)
			tt.setupMock(mockAuthManager)

			// Create test command.
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String(IdentityFlagName, "", "identity")
			if tt.identityFlag != "" {
				_ = cmd.Flags().Set(IdentityFlagName, tt.identityFlag)
			}

			// Set up viper.
			v := viper.New()
			if tt.viperIdentity != "" {
				v.Set(IdentityFlagName, tt.viperIdentity)
			}

			result, err := resolveIdentityName(cmd, v, mockAuthManager)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestRetrieveCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name          string
		whoami        *authTypes.WhoamiInfo
		expectedError error
	}{
		{
			name: "credentials directly in whoami",
			whoami: &authTypes.WhoamiInfo{
				Credentials: authTypes.NewMockICredentials(ctrl),
			},
			expectedError: nil,
		},
		{
			name: "no credentials available",
			whoami: &authTypes.WhoamiInfo{
				Credentials:    nil,
				CredentialsRef: "",
			},
			expectedError: errUtils.ErrAuthConsole,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds, err := retrieveCredentials(tt.whoami)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
				assert.Nil(t, creds)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, creds)
			}
		})
	}
}

func TestResolveConsoleDuration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name           string
		setupMock      func(*authTypes.MockAuthManager)
		flagChanged    bool
		flagDuration   time.Duration
		providerName   string
		expectedResult time.Duration
		expectedError  error
	}{
		{
			name: "flag takes precedence",
			setupMock: func(m *authTypes.MockAuthManager) {
				// No mock calls needed when flag is explicitly set.
			},
			flagChanged:    true,
			flagDuration:   2 * time.Hour,
			providerName:   "aws-sso",
			expectedResult: 2 * time.Hour,
			expectedError:  nil,
		},
		{
			name: "uses provider config when flag not set",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{
					"aws-sso": {
						Console: &schema.ConsoleConfig{
							SessionDuration: "4h",
						},
					},
				})
			},
			flagChanged:    false,
			flagDuration:   1 * time.Hour,
			providerName:   "aws-sso",
			expectedResult: 4 * time.Hour,
			expectedError:  nil,
		},
		{
			name: "falls back to flag default when no provider config",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{
					"aws-sso": {},
				})
			},
			flagChanged:    false,
			flagDuration:   1 * time.Hour,
			providerName:   "aws-sso",
			expectedResult: 1 * time.Hour,
			expectedError:  nil,
		},
		{
			name: "falls back when provider not found",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{})
			},
			flagChanged:    false,
			flagDuration:   1 * time.Hour,
			providerName:   "nonexistent",
			expectedResult: 1 * time.Hour,
			expectedError:  nil,
		},
		{
			name: "invalid provider duration format",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetProviders().Return(map[string]schema.Provider{
					"aws-sso": {
						Console: &schema.ConsoleConfig{
							SessionDuration: "invalid",
						},
					},
				})
			},
			flagChanged:    false,
			flagDuration:   1 * time.Hour,
			providerName:   "aws-sso",
			expectedResult: 0,
			expectedError:  nil, // Function returns error, check non-nil.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAuthManager := authTypes.NewMockAuthManager(ctrl)
			tt.setupMock(mockAuthManager)

			// Create test command with duration flag.
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().Duration("duration", 1*time.Hour, "duration")
			if tt.flagChanged {
				_ = cmd.Flags().Set("duration", tt.flagDuration.String())
			}

			result, err := resolveConsoleDuration(cmd, mockAuthManager, tt.providerName, tt.flagDuration)

			switch {
			case tt.name == "invalid provider duration format":
				// This case returns an error.
				assert.Error(t, err)
			case tt.expectedError != nil:
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			default:
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestGetConsoleProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name          string
		setupMock     func(*authTypes.MockAuthManager)
		identityName  string
		expectedError error
	}{
		{
			name: "AWS IAM Identity Center provider",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetProviderKindForIdentity("prod").Return(authTypes.ProviderKindAWSIAMIdentityCenter, nil)
			},
			identityName:  "prod",
			expectedError: nil,
		},
		{
			name: "AWS SAML provider",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetProviderKindForIdentity("prod").Return(authTypes.ProviderKindAWSSAML, nil)
			},
			identityName:  "prod",
			expectedError: nil,
		},
		{
			name: "Azure OIDC provider",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetProviderKindForIdentity("azure-prod").Return(authTypes.ProviderKindAzureOIDC, nil)
			},
			identityName:  "azure-prod",
			expectedError: nil,
		},
		{
			name: "Azure CLI provider",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetProviderKindForIdentity("azure-cli").Return(authTypes.ProviderKindAzureCLI, nil)
			},
			identityName:  "azure-cli",
			expectedError: nil,
		},
		{
			name: "Azure Device Code provider",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetProviderKindForIdentity("azure-device").Return(authTypes.ProviderKindAzureDeviceCode, nil)
			},
			identityName:  "azure-device",
			expectedError: nil,
		},
		{
			name: "GCP OIDC provider not supported",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetProviderKindForIdentity("gcp-prod").Return(authTypes.ProviderKindGCPOIDC, nil)
			},
			identityName:  "gcp-prod",
			expectedError: errUtils.ErrProviderNotSupported,
		},
		{
			name: "unknown provider kind",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetProviderKindForIdentity("unknown").Return("unknown-kind", nil)
			},
			identityName:  "unknown",
			expectedError: errUtils.ErrProviderNotSupported,
		},
		{
			name: "error getting provider kind",
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetProviderKindForIdentity("error").Return("", errUtils.ErrProviderNotFound)
			},
			identityName:  "error",
			expectedError: nil, // The error is wrapped, not a direct match.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAuthManager := authTypes.NewMockAuthManager(ctrl)
			tt.setupMock(mockAuthManager)

			provider, err := getConsoleProvider(mockAuthManager, tt.identityName)

			switch {
			case tt.expectedError != nil:
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
				assert.Nil(t, provider)
			case tt.name == "error getting provider kind":
				assert.Error(t, err)
			default:
				assert.NoError(t, err)
				assert.NotNil(t, provider)
			}
		})
	}
}

func TestAuthConsoleCommand_Structure(t *testing.T) {
	assert.Equal(t, "console", authConsoleCmd.Use)
	assert.NotEmpty(t, authConsoleCmd.Short)
	assert.NotEmpty(t, authConsoleCmd.Long)
	assert.NotEmpty(t, authConsoleCmd.Example)
	assert.NotNil(t, authConsoleCmd.RunE)

	// Check flags exist.
	destinationFlag := authConsoleCmd.Flags().Lookup("destination")
	assert.NotNil(t, destinationFlag)

	durationFlag := authConsoleCmd.Flags().Lookup("duration")
	assert.NotNil(t, durationFlag)

	issuerFlag := authConsoleCmd.Flags().Lookup("issuer")
	assert.NotNil(t, issuerFlag)

	printOnlyFlag := authConsoleCmd.Flags().Lookup("print-only")
	assert.NotNil(t, printOnlyFlag)

	noOpenFlag := authConsoleCmd.Flags().Lookup("no-open")
	assert.NotNil(t, noOpenFlag)
}

func TestConsoleParser_Initialization(t *testing.T) {
	// consoleParser should be initialized in init().
	assert.NotNil(t, consoleParser)
}

func TestConsoleLabelWidth(t *testing.T) {
	assert.Equal(t, 18, ConsoleLabelWidth)
}

func TestConsoleOutputFormat(t *testing.T) {
	assert.Equal(t, "%s %s\n", ConsoleOutputFormat)
}

func TestDestinationFlagCompletion(t *testing.T) {
	completions, directive := destinationFlagCompletion(nil, nil, "")

	// Should return AWS service aliases.
	assert.NotEmpty(t, completions)
	// Check some expected aliases.
	assert.Contains(t, completions, "s3")
	assert.Contains(t, completions, "ec2")
	// Directive should indicate no file completion.
	assert.NotZero(t, directive)
}
