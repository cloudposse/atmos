package auth

import (
	"context"
	"errors"
	"testing"

	charm "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stubAuthManager implements types.AuthManager for focused unit tests.
type stubAuthManager struct {
	defaultIdentity string
	defaultErr      error
	whoami          *types.WhoamiInfo
	envVars         map[string]string // Environment variables to return from GetEnvironmentVariables
}

func (s *stubAuthManager) Authenticate(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	return s.whoami, nil
}

func (s *stubAuthManager) Whoami(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	return s.whoami, nil
}

func (s *stubAuthManager) GetCachedCredentials(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	return s.whoami, nil
}

func (s *stubAuthManager) Validate() error { return nil }
func (s *stubAuthManager) GetDefaultIdentity() (string, error) {
	return s.defaultIdentity, s.defaultErr
}
func (s *stubAuthManager) ListIdentities() []string                          { return []string{"one", "two"} }
func (s *stubAuthManager) GetProviderForIdentity(identityName string) string { return "prov" }
func (s *stubAuthManager) GetFilesDisplayPath(providerName string) string    { return "~/.aws/atmos" }
func (s *stubAuthManager) GetProviderKindForIdentity(identityName string) (string, error) {
	return "kind", nil
}
func (s *stubAuthManager) GetChain() []string { return []string{"prov", "id"} }
func (s *stubAuthManager) GetStackInfo() *schema.ConfigAndStacksInfo {
	return &schema.ConfigAndStacksInfo{}
}
func (s *stubAuthManager) ListProviders() []string { return []string{"prov"} }
func (s *stubAuthManager) GetIdentities() map[string]schema.Identity {
	return map[string]schema.Identity{}
}

func (s *stubAuthManager) GetProviders() map[string]schema.Provider {
	return map[string]schema.Provider{}
}

func (s *stubAuthManager) Logout(ctx context.Context, identityName string) error {
	return nil
}

func (s *stubAuthManager) LogoutProvider(ctx context.Context, providerName string) error {
	return nil
}

func (s *stubAuthManager) LogoutAll(ctx context.Context) error {
	return nil
}

func (s *stubAuthManager) GetEnvironmentVariables(identityName string) (map[string]string, error) {
	if s.envVars != nil {
		return s.envVars, nil
	}
	return make(map[string]string), nil
}

func TestGetConfigLogLevels(t *testing.T) {
	tests := []struct {
		name             string
		atmosLogLevel    string
		authLogLevel     string
		setupGlobalLevel charm.Level // Set global log level before calling getConfigLogLevels
		expectedAtmosStr string
		expectedAuthStr  string
	}{
		{
			name:             "nil config falls back to Info",
			setupGlobalLevel: charm.InfoLevel,
			expectedAtmosStr: "info",
			expectedAuthStr:  "info",
		},
		{
			name:             "empty config falls back to current global level",
			setupGlobalLevel: charm.WarnLevel,
			atmosLogLevel:    "",
			authLogLevel:     "",
			expectedAtmosStr: "warn",
			expectedAuthStr:  "warn",
		},
		{
			name:             "exact case Debug",
			setupGlobalLevel: charm.DebugLevel,
			atmosLogLevel:    "Debug",
			authLogLevel:     "",
			expectedAtmosStr: "debug",
			expectedAuthStr:  "debug",
		},
		{
			name:             "lowercase warning",
			setupGlobalLevel: charm.WarnLevel,
			atmosLogLevel:    "Warning",
			authLogLevel:     "warning",
			expectedAtmosStr: "warn",
			expectedAuthStr:  "warn",
		},
		{
			name:             "uppercase WARN",
			setupGlobalLevel: charm.WarnLevel,
			atmosLogLevel:    "Warning",
			authLogLevel:     "WARN",
			expectedAtmosStr: "warn",
			expectedAuthStr:  "warn",
		},
		{
			name:             "mixed case WaRnInG",
			setupGlobalLevel: charm.WarnLevel,
			atmosLogLevel:    "Warning",
			authLogLevel:     "WaRnInG",
			expectedAtmosStr: "warn",
			expectedAuthStr:  "warn",
		},
		{
			name:             "warn alias",
			setupGlobalLevel: charm.WarnLevel,
			atmosLogLevel:    "Warning",
			authLogLevel:     "warn",
			expectedAtmosStr: "warn",
			expectedAuthStr:  "warn",
		},
		{
			name:             "auth overrides atmos level",
			setupGlobalLevel: charm.DebugLevel,
			atmosLogLevel:    "Debug",
			authLogLevel:     "Error",
			expectedAtmosStr: "debug",
			expectedAuthStr:  "error",
		},
		{
			name:             "trace level",
			setupGlobalLevel: log.TraceLevel,
			atmosLogLevel:    "Trace",
			authLogLevel:     "trace",
			expectedAtmosStr: "trace",
			expectedAuthStr:  "trace",
		},
		{
			name:             "off level",
			setupGlobalLevel: charm.FatalLevel,
			atmosLogLevel:    "Off",
			authLogLevel:     "Off",
			expectedAtmosStr: "fatal",
			expectedAuthStr:  "fatal",
		},
		{
			name:             "invalid auth level falls back to atmos level",
			setupGlobalLevel: charm.WarnLevel,
			atmosLogLevel:    "Warning",
			authLogLevel:     "InvalidLevel",
			expectedAtmosStr: "warn",
			expectedAuthStr:  "warn", // Falls back to atmos level
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up global log level to simulate what setupLogger() does.
			log.SetLevel(tt.setupGlobalLevel)

			var cfg *schema.AtmosConfiguration
			if tt.name == "nil config falls back to Info" {
				cfg = nil
			} else {
				cfg = &schema.AtmosConfiguration{
					Logs: schema.Logs{
						Level: tt.atmosLogLevel,
					},
					Auth: schema.AuthConfig{
						Logs: schema.Logs{
							Level: tt.authLogLevel,
						},
					},
				}
			}

			atmos, auth := getConfigLogLevels(cfg)

			assert.Equal(t, tt.expectedAtmosStr, log.LevelToString(atmos),
				"Atmos log level mismatch for config: atmos=%q, auth=%q", tt.atmosLogLevel, tt.authLogLevel)
			assert.Equal(t, tt.expectedAuthStr, log.LevelToString(auth),
				"Auth log level mismatch for config: atmos=%q, auth=%q", tt.atmosLogLevel, tt.authLogLevel)
		})
	}
}

func TestDecodeAuthConfigFromStack(t *testing.T) {
	// Success with minimal providers/identities map.
	stack := &schema.ConfigAndStacksInfo{
		ComponentAuthSection: schema.AtmosSectionMapType{
			"providers": map[string]any{
				"aws-sso": map[string]any{
					"kind":      "aws/iam-identity-center",
					"region":    "us-east-1",
					"start_url": "https://example.awsapps.com/start",
				},
			},
			"identities": map[string]any{
				"dev": map[string]any{
					"kind": "aws/permission-set",
					"via": map[string]any{
						"provider": "aws-sso",
					},
					"principal": map[string]any{
						"name": "Developer",
						"account": map[string]any{
							"name": "dev",
						},
					},
				},
			},
		},
	}
	cfg, err := decodeAuthConfigFromStack(stack)
	assert.NoError(t, err)
	assert.Contains(t, cfg.Providers, "aws-sso")
	assert.Contains(t, cfg.Identities, "dev")

	// Invalid type should surface ErrInvalidAuthConfig.
	bad := &schema.ConfigAndStacksInfo{ComponentAuthSection: schema.AtmosSectionMapType{"providers": 42}}
	_, err = decodeAuthConfigFromStack(bad)
	assert.Error(t, err)
}

func TestResolveTargetIdentityName(t *testing.T) {
	// Directly specified on stack wins.
	stack := &schema.ConfigAndStacksInfo{Identity: "explicit"}
	name, err := resolveTargetIdentityName(stack, &stubAuthManager{defaultIdentity: "default"})
	assert.NoError(t, err)
	assert.Equal(t, "explicit", name)

	// Fallback to manager default.
	stack.Identity = ""
	name, err = resolveTargetIdentityName(stack, &stubAuthManager{defaultIdentity: "team"})
	assert.NoError(t, err)
	assert.Equal(t, "team", name)

	// Manager error returns ErrDefaultIdentity.
	_, err = resolveTargetIdentityName(stack, &stubAuthManager{defaultErr: errors.New("boom")})
	assert.Error(t, err)

	// Manager returns empty default -> ErrNoDefaultIdentity.
	_, err = resolveTargetIdentityName(stack, &stubAuthManager{defaultIdentity: ""})
	assert.Error(t, err)
}

func TestAuthenticateAndWriteEnv(t *testing.T) {
	m := &stubAuthManager{whoami: &types.WhoamiInfo{Provider: "p", Identity: "i"}}
	atmosCfg := &schema.AtmosConfiguration{}
	stack := &schema.ConfigAndStacksInfo{ComponentEnvSection: schema.AtmosSectionMapType{"FOO": "BAR"}}
	err := authenticateAndWriteEnv(context.Background(), m, "dev", atmosCfg, stack)
	assert.NoError(t, err)
}

func TestAuthenticateAndWriteEnv_AddsEnvironmentVariables(t *testing.T) {
	tests := []struct {
		name              string
		envVars           map[string]string
		initialEnvSection schema.AtmosSectionMapType
		expectedKeys      []string
	}{
		{
			name: "adds AWS environment variables to empty section",
			envVars: map[string]string{
				"AWS_CONFIG_FILE":             "/path/to/config",
				"AWS_SHARED_CREDENTIALS_FILE": "/path/to/creds",
				"AWS_PROFILE":                 "my-profile",
				"AWS_REGION":                  "us-east-1",
				"AWS_EC2_METADATA_DISABLED":   "true",
			},
			initialEnvSection: nil,
			expectedKeys:      []string{"AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE", "AWS_PROFILE", "AWS_REGION", "AWS_EC2_METADATA_DISABLED"},
		},
		{
			name: "merges with existing environment variables",
			envVars: map[string]string{
				"AWS_PROFILE": "my-profile",
				"AWS_REGION":  "us-west-2",
			},
			initialEnvSection: schema.AtmosSectionMapType{
				"EXISTING_VAR": "value",
				"TF_VAR_foo":   "bar",
			},
			expectedKeys: []string{"EXISTING_VAR", "TF_VAR_foo", "AWS_PROFILE", "AWS_REGION"},
		},
		{
			name:              "handles no environment variables from identity",
			envVars:           map[string]string{},
			initialEnvSection: schema.AtmosSectionMapType{"FOO": "BAR"},
			expectedKeys:      []string{"FOO"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create stub manager that returns our test env vars.
			stub := &stubAuthManager{
				whoami: &types.WhoamiInfo{Provider: "test-provider", Identity: "test-identity"},
			}
			// Override GetEnvironmentVariables to return test data.
			stub.envVars = tt.envVars

			atmosCfg := &schema.AtmosConfiguration{}
			stack := &schema.ConfigAndStacksInfo{
				ComponentEnvSection: tt.initialEnvSection,
			}

			err := authenticateAndWriteEnv(context.Background(), stub, "test-identity", atmosCfg, stack)
			assert.NoError(t, err)

			// Verify all expected keys are present in ComponentEnvSection.
			assert.NotNil(t, stack.ComponentEnvSection, "ComponentEnvSection should not be nil")
			for _, key := range tt.expectedKeys {
				assert.Contains(t, stack.ComponentEnvSection, key, "ComponentEnvSection should contain %s", key)
			}

			// Verify the values match what we provided.
			for k, expectedValue := range tt.envVars {
				actualValue, exists := stack.ComponentEnvSection[k]
				assert.True(t, exists, "Expected key %s to exist in ComponentEnvSection", k)
				assert.Equal(t, expectedValue, actualValue, "Value mismatch for key %s", k)
			}
		})
	}
}

func TestTerraformPreHook_NoAuthConfigEarlyExit(t *testing.T) {
	atmosCfg := &schema.AtmosConfiguration{}
	stack := &schema.ConfigAndStacksInfo{ComponentAuthSection: schema.AtmosSectionMapType{}}
	err := TerraformPreHook(atmosCfg, stack)
	assert.NoError(t, err)
}

func TestTerraformPreHook_InvalidAuthConfig(t *testing.T) {
	atmosCfg := &schema.AtmosConfiguration{}
	// Malformed auth section.
	stack := &schema.ConfigAndStacksInfo{ComponentAuthSection: schema.AtmosSectionMapType{"providers": 42}}
	err := TerraformPreHook(atmosCfg, stack)
	assert.Error(t, err)
}
