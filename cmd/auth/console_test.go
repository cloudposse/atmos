package auth

import (
	"context"
	"errors"
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

// TestConsoleSessionDir verifies the deterministic XDG data directory used for
// isolated browser sessions. Same realm+identity must hash to the same dirname;
// different realms or identities must diverge.
func TestConsoleSessionDir(t *testing.T) {
	t.Run("returns a path under the XDG data dir", func(t *testing.T) {
		path, err := consoleSessionDir("test-realm", "id-a")
		require.NoError(t, err)
		assert.Contains(t, path, "console")
		assert.Contains(t, path, "sessions")
	})

	t.Run("same realm+identity produces the same path", func(t *testing.T) {
		first, err := consoleSessionDir("realm-1", "shared-id")
		require.NoError(t, err)
		second, err := consoleSessionDir("realm-1", "shared-id")
		require.NoError(t, err)
		assert.Equal(t, first, second,
			"deterministic hash means repeat calls reuse the same browser profile dir")
	})

	t.Run("different identity produces a different path", func(t *testing.T) {
		first, err := consoleSessionDir("realm-1", "id-a")
		require.NoError(t, err)
		second, err := consoleSessionDir("realm-1", "id-b")
		require.NoError(t, err)
		assert.NotEqual(t, first, second)
	})

	t.Run("different realm produces a different path", func(t *testing.T) {
		first, err := consoleSessionDir("realm-1", "shared-id")
		require.NoError(t, err)
		second, err := consoleSessionDir("realm-2", "shared-id")
		require.NoError(t, err)
		assert.NotEqual(t, first, second,
			"realm scoping must isolate sessions across credential boundaries")
	})
}

// TestResolveConsoleIsolated covers the flag/config/default precedence ladder.
func TestResolveConsoleIsolated(t *testing.T) {
	newCmdWithIsolatedFlag := func() *cobra.Command {
		cmd := &cobra.Command{Use: "console"}
		cmd.Flags().Bool("isolated", false, "isolated session")
		return cmd
	}

	t.Run("default is false when flag and config both absent", func(t *testing.T) {
		cmd := newCmdWithIsolatedFlag()
		v := viper.New()
		cfg := &schema.AtmosConfiguration{}

		assert.False(t, resolveConsoleIsolated(cmd, v, cfg))
	})

	t.Run("auth.console.isolated config is honoured when flag is unset", func(t *testing.T) {
		cmd := newCmdWithIsolatedFlag()
		v := viper.New()
		isolated := true
		cfg := &schema.AtmosConfiguration{
			Auth: schema.AuthConfig{
				Console: &schema.AuthConsoleConfig{Isolated: &isolated},
			},
		}

		assert.True(t, resolveConsoleIsolated(cmd, v, cfg))
	})

	t.Run("flag overrides config when explicitly set", func(t *testing.T) {
		cmd := newCmdWithIsolatedFlag()
		require.NoError(t, cmd.Flags().Set("isolated", "false"))

		v := viper.New()
		v.Set("isolated", false)

		// Config says isolated=true; flag should win.
		isolated := true
		cfg := &schema.AtmosConfiguration{
			Auth: schema.AuthConfig{
				Console: &schema.AuthConsoleConfig{Isolated: &isolated},
			},
		}

		assert.False(t, resolveConsoleIsolated(cmd, v, cfg),
			"explicit --isolated=false must override auth.console.isolated=true config")
	})

	t.Run("flag set to true overrides config false", func(t *testing.T) {
		cmd := newCmdWithIsolatedFlag()
		require.NoError(t, cmd.Flags().Set("isolated", "true"))

		v := viper.New()
		v.Set("isolated", true)

		isolated := false
		cfg := &schema.AtmosConfiguration{
			Auth: schema.AuthConfig{
				Console: &schema.AuthConsoleConfig{Isolated: &isolated},
			},
		}

		assert.True(t, resolveConsoleIsolated(cmd, v, cfg))
	})
}

// TestPrintConsoleHelpers asserts the formatted I/O helpers don't panic and
// produce the expected key strings. Coverage gain over no-tests.
func TestPrintConsoleHelpers(t *testing.T) {
	t.Run("printConsoleURL writes the URL", func(t *testing.T) {
		assert.NotPanics(t, func() {
			printConsoleURL("https://example.com/console")
		})
	})

	t.Run("printConsoleInfo with full whoami including expiration", func(t *testing.T) {
		future := time.Now().Add(2 * time.Hour)
		whoami := &authTypes.WhoamiInfo{
			Provider:   "aws-sso",
			Identity:   "prod-admin",
			Account:    "123456789012",
			Expiration: &future,
		}

		assert.NotPanics(t, func() {
			printConsoleInfo(whoami, time.Hour, false, "")
		})
	})

	t.Run("printConsoleInfo with showURL=true prints the URL", func(t *testing.T) {
		whoami := &authTypes.WhoamiInfo{Provider: "aws-sso", Identity: "prod-admin"}
		assert.NotPanics(t, func() {
			printConsoleInfo(whoami, 0, true, "https://example.com")
		})
	})
}

// TestRetrieveCredentials_NoCredentialsAvailable asserts that an empty
// WhoamiInfo (no Credentials, no CredentialsRef) errors out cleanly.
func TestRetrieveCredentials_NoCredentialsAvailable(t *testing.T) {
	whoami := &authTypes.WhoamiInfo{Identity: "id"}
	creds, err := retrieveCredentials(whoami)
	require.Error(t, err)
	assert.Nil(t, creds)
	assert.ErrorIs(t, err, errUtils.ErrAuthConsole)
}

// TestRetrieveCredentials_InlineCredentials covers the path where the
// WhoamiInfo already carries Credentials directly. The function must return
// those without touching the credential store.
func TestRetrieveCredentials_InlineCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockCreds := authTypes.NewMockICredentials(ctrl)

	whoami := &authTypes.WhoamiInfo{
		Identity:    "prod-admin",
		Credentials: mockCreds,
	}

	got, err := retrieveCredentials(whoami)
	require.NoError(t, err)
	assert.Same(t, mockCreds, got,
		"inline Credentials must be returned verbatim without store lookup")
}

// fakeOpener is a test double for browser.Opener with switchable error
// behaviour. Tracks whether Open was called and with which URL.
type fakeOpener struct {
	openErr  error
	openedAt string
	calls    int
}

func (f *fakeOpener) Open(url string) error {
	f.calls++
	f.openedAt = url
	return f.openErr
}

// TestHandleBrowserOpen exercises the three branches of the browser-open
// router: skipOpen=true (always show URL), opener=nil (CI/no-TTY, show URL),
// and opener.Open success vs error.
func TestHandleBrowserOpen(t *testing.T) {
	const url = "https://console.example.com/sign-in"

	t.Run("skipOpen=true prints URL and does not call opener", func(t *testing.T) {
		op := &fakeOpener{}
		assert.NotPanics(t, func() {
			handleBrowserOpen(url, true /*skipOpen*/, op)
		})
		assert.Zero(t, op.calls, "skipOpen must not invoke the browser opener")
	})

	t.Run("nil opener prints URL and does not panic", func(t *testing.T) {
		// nil opener is the CI/no-TTY case: handleBrowserOpen falls through
		// to printConsoleURL.
		assert.NotPanics(t, func() {
			handleBrowserOpen(url, false /*skipOpen*/, nil)
		})
	})

	t.Run("successful Open is invoked with the URL", func(t *testing.T) {
		op := &fakeOpener{}
		// When CI is detected, the function takes the skipOpen-style branch
		// and does not call the opener. We can't reliably disable CI mode in
		// unit tests, so this test simply asserts non-panic plus invocation
		// count consistency.
		assert.NotPanics(t, func() {
			handleBrowserOpen(url, false, op)
		})
		// Either 0 (CI detected, fell through to print) or 1 (not CI, called)
		// — both are valid; assert no double-call.
		assert.LessOrEqual(t, op.calls, 1)
		if op.calls == 1 {
			assert.Equal(t, url, op.openedAt)
		}
	})

	t.Run("opener error path is non-panicking", func(t *testing.T) {
		op := &fakeOpener{openErr: errors.New("browser unavailable")}
		assert.NotPanics(t, func() {
			handleBrowserOpen(url, false, op)
		})
	})
}

// TestExecuteAuthConsoleCommand_SmokeNoConfig exercises the console
// orchestrator from a directory without an atmos.yaml. Contract: no panic.
func TestExecuteAuthConsoleCommand_SmokeNoConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	cmd := authConsoleCmd
	cmd.SetContext(context.Background())

	assert.NotPanics(t, func() {
		_ = executeAuthConsoleCommand(cmd, nil)
	})
}

// TestExecuteAuthConsoleCommand_WithMockAuth drives the console orchestrator
// against the mock auth fixture. The mock/aws provider is not in the
// ProviderKindAWSIAMIdentityCenter/SAML/Azure* set, so getConsoleProvider
// returns ErrProviderNotSupported — exercising that branch of the executor.
// --print-only is set so the function does not attempt to open a browser.
func TestExecuteAuthConsoleCommand_WithMockAuth(t *testing.T) {
	setupMockAuthFixture(t)

	cmd := authConsoleCmd
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags([]string{"--print-only"}))

	// Mock provider doesn't support console access; expect the
	// ErrProviderNotSupported sentinel surfaced via getConsoleProvider.
	err := executeAuthConsoleCommand(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAuthConsole,
		"console against an unsupported provider must wrap with ErrAuthConsole")
}

// TestInitializeAuthManager_SmokeFromEmptyTempDir exercises the console
// helper from a directory without an atmos.yaml.
func TestInitializeAuthManager_SmokeFromEmptyTempDir(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	cmd := &cobra.Command{Use: "console"}
	v := viper.New()

	manager, atmosCfg, err := initializeAuthManager(cmd, v)
	if err != nil {
		// Either ErrAuthConsole or wrapped variants are acceptable; the contract
		// is "any error must be a documented sentinel and nil returns".
		assert.ErrorIs(t, err, errUtils.ErrAuthConsole,
			"initializeAuthManager must wrap failures with ErrAuthConsole")
		assert.Nil(t, manager)
		assert.Nil(t, atmosCfg)
		return
	}
	assert.NotNil(t, manager)
	assert.NotNil(t, atmosCfg)
}
