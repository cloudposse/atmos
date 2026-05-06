package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stubAuthManager implements types.AuthManager for focused unit tests.
type stubAuthManager struct {
	defaultIdentity string
	defaultErr      error
	whoami          *types.WhoamiInfo
	envVars         map[string]string          // Environment variables to return from GetEnvironmentVariables.
	identities      map[string]schema.Identity // Identities to return from GetIdentities.

	// Profile-fallback instrumentation.
	fallbackCalls int   // Number of times MaybeOfferAnyProfileFallback was invoked.
	fallbackErr   error // Error to return from MaybeOfferAnyProfileFallback.
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

func (s *stubAuthManager) AuthenticateProvider(ctx context.Context, providerName string) (*types.WhoamiInfo, error) {
	return nil, nil
}

func (s *stubAuthManager) Validate() error { return nil }
func (s *stubAuthManager) GetDefaultIdentity(_ bool) (string, error) {
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
	if s.identities != nil {
		return s.identities
	}
	return map[string]schema.Identity{}
}

func (s *stubAuthManager) GetProviders() map[string]schema.Provider {
	return map[string]schema.Provider{}
}

func (s *stubAuthManager) Logout(ctx context.Context, identityName string, deleteKeychain bool) error {
	return nil
}

func (s *stubAuthManager) LogoutProvider(ctx context.Context, providerName string, deleteKeychain bool) error {
	return nil
}

func (s *stubAuthManager) LogoutAll(ctx context.Context, deleteKeychain bool) error {
	return nil
}

func (s *stubAuthManager) GetEnvironmentVariables(identityName string) (map[string]string, error) {
	if s.envVars != nil {
		return s.envVars, nil
	}
	return make(map[string]string), nil
}

func (s *stubAuthManager) PrepareShellEnvironment(ctx context.Context, identityName string, currentEnv []string) ([]string, error) {
	// Merge envVars into currentEnv.
	// This simulates what the real PrepareShellEnvironment does.
	envMap := make(map[string]string)

	// Parse currentEnv into map.
	for _, envVar := range currentEnv {
		if idx := strings.IndexByte(envVar, '='); idx >= 0 {
			key := envVar[:idx]
			value := envVar[idx+1:]
			envMap[key] = value
		}
	}

	// Merge in envVars from stub.
	for k, v := range s.envVars {
		envMap[k] = v
	}

	// Convert back to list.
	result := make([]string, 0, len(envMap))
	for k, v := range envMap {
		result = append(result, k+"="+v)
	}

	return result, nil
}

func (s *stubAuthManager) ExecuteIntegration(ctx context.Context, integrationName string) error {
	return nil
}

func (s *stubAuthManager) ExecuteIdentityIntegrations(ctx context.Context, identityName string) error {
	return nil
}

func (s *stubAuthManager) GetIntegration(integrationName string) (*schema.Integration, error) {
	return nil, nil
}

func (s *stubAuthManager) ResolvePrincipalSetting(identityName, key string) (interface{}, bool) {
	return nil, false
}

func (s *stubAuthManager) ResolveProviderConfig(identityName string) (*schema.Provider, bool) {
	return nil, false
}

func (s *stubAuthManager) MaybeOfferAnyProfileFallback(_ context.Context) error {
	s.fallbackCalls++
	return s.fallbackErr
}

func (s *stubAuthManager) GetRealm() realm.RealmInfo {
	return realm.RealmInfo{}
}

func TestGetConfigLogLevels(t *testing.T) {
	tests := []struct {
		name             string
		atmosLogLevel    string
		authLogLevel     string
		setupGlobalLevel log.Level // Set global log level before calling getConfigLogLevels.
		expectedAtmosStr string
		expectedAuthStr  string
	}{
		{
			name:             "nil config falls back to Info",
			setupGlobalLevel: log.InfoLevel,
			expectedAtmosStr: "info",
			expectedAuthStr:  "info",
		},
		{
			name:             "empty config falls back to current global level",
			setupGlobalLevel: log.WarnLevel,
			atmosLogLevel:    "",
			authLogLevel:     "",
			expectedAtmosStr: "warn",
			expectedAuthStr:  "warn",
		},
		{
			name:             "exact case Debug",
			setupGlobalLevel: log.DebugLevel,
			atmosLogLevel:    "Debug",
			authLogLevel:     "",
			expectedAtmosStr: "debug",
			expectedAuthStr:  "debug",
		},
		{
			name:             "lowercase warning",
			setupGlobalLevel: log.WarnLevel,
			atmosLogLevel:    "Warning",
			authLogLevel:     "warning",
			expectedAtmosStr: "warn",
			expectedAuthStr:  "warn",
		},
		{
			name:             "uppercase WARN",
			setupGlobalLevel: log.WarnLevel,
			atmosLogLevel:    "Warning",
			authLogLevel:     "WARN",
			expectedAtmosStr: "warn",
			expectedAuthStr:  "warn",
		},
		{
			name:             "mixed case WaRnInG",
			setupGlobalLevel: log.WarnLevel,
			atmosLogLevel:    "Warning",
			authLogLevel:     "WaRnInG",
			expectedAtmosStr: "warn",
			expectedAuthStr:  "warn",
		},
		{
			name:             "warn alias",
			setupGlobalLevel: log.WarnLevel,
			atmosLogLevel:    "Warning",
			authLogLevel:     "warn",
			expectedAtmosStr: "warn",
			expectedAuthStr:  "warn",
		},
		{
			name:             "auth overrides atmos level",
			setupGlobalLevel: log.DebugLevel,
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
			setupGlobalLevel: log.FatalLevel,
			atmosLogLevel:    "Off",
			authLogLevel:     "Off",
			expectedAtmosStr: "fatal",
			expectedAuthStr:  "fatal",
		},
		{
			name:             "invalid auth level falls back to atmos level",
			setupGlobalLevel: log.WarnLevel,
			atmosLogLevel:    "Warning",
			authLogLevel:     "InvalidLevel",
			expectedAtmosStr: "warn",
			expectedAuthStr:  "warn", // Falls back to atmos level.
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
	ctx := context.Background()

	// Directly specified on stack wins.
	stack := &schema.ConfigAndStacksInfo{Identity: "explicit"}
	mgr := &stubAuthManager{defaultIdentity: "default"}
	name, err := resolveTargetIdentityName(ctx, stack, mgr)
	assert.NoError(t, err)
	assert.Equal(t, "explicit", name)
	assert.Zero(t, mgr.fallbackCalls, "explicit --identity must skip the profile fallback")

	// Fallback to manager default.
	stack.Identity = ""
	mgr = &stubAuthManager{defaultIdentity: "team"}
	name, err = resolveTargetIdentityName(ctx, stack, mgr)
	assert.NoError(t, err)
	assert.Equal(t, "team", name)
	assert.Zero(t, mgr.fallbackCalls, "successful default identity resolution must skip the profile fallback")

	// Manager error returns ErrDefaultIdentity.
	mgr = &stubAuthManager{defaultErr: errors.New("boom")}
	_, err = resolveTargetIdentityName(ctx, stack, mgr)
	assert.Error(t, err)
	assert.Zero(t, mgr.fallbackCalls, "unrelated errors must not trigger the profile fallback")

	// Manager returns empty default -> ErrNoDefaultIdentity.
	_, err = resolveTargetIdentityName(ctx, stack, &stubAuthManager{defaultIdentity: ""})
	assert.Error(t, err)
}

// TestResolveTargetIdentityName_InvokesFallbackOnNoAuthConfig is a regression
// guard: when GetDefaultIdentity returns a "no auth config" terminal error
// (ErrNoDefaultIdentity, ErrNoIdentitiesAvailable, or ErrNoProvidersAvailable),
// resolveTargetIdentityName MUST invoke authManager.MaybeOfferAnyProfileFallback
// before surfacing the fatal error. Without this, terraform commands never
// trigger the profile-switch prompt that auth commands already support.
func TestResolveTargetIdentityName_InvokesFallbackOnNoAuthConfig(t *testing.T) {
	ctx := context.Background()
	stack := &schema.ConfigAndStacksInfo{}

	triggers := []struct {
		name string
		err  error
	}{
		{"ErrNoDefaultIdentity", errUtils.ErrNoDefaultIdentity},
		{"ErrNoIdentitiesAvailable", errUtils.ErrNoIdentitiesAvailable},
		{"ErrNoProvidersAvailable", errUtils.ErrNoProvidersAvailable},
	}
	for _, tc := range triggers {
		t.Run(tc.name, func(t *testing.T) {
			mgr := &stubAuthManager{defaultErr: tc.err}
			_, err := resolveTargetIdentityName(ctx, stack, mgr)
			assert.Error(t, err)
			assert.Equal(t, 1, mgr.fallbackCalls,
				"MaybeOfferAnyProfileFallback must fire for %s", tc.name)
		})
	}
}

// TestResolveTargetIdentityName_SurfacesFallbackError verifies that when
// MaybeOfferAnyProfileFallback returns an enriched error (non-interactive
// branch with candidate profiles), resolveTargetIdentityName surfaces that
// error instead of the original bare ErrNoDefaultIdentity.
func TestResolveTargetIdentityName_SurfacesFallbackError(t *testing.T) {
	ctx := context.Background()
	stack := &schema.ConfigAndStacksInfo{}
	enriched := errors.New("try --profile managers")
	mgr := &stubAuthManager{
		defaultErr:  errUtils.ErrNoDefaultIdentity,
		fallbackErr: enriched,
	}

	_, err := resolveTargetIdentityName(ctx, stack, mgr)
	assert.ErrorIs(t, err, enriched,
		"an enriched fallback error must be surfaced to the caller instead of the original")
}

// TestResolveTargetIdentityName_DoesNotInvokeFallbackOnUnrelatedError is the
// negative-path guard: recovery logic must not trigger for errors that are
// unrelated to missing auth config.
func TestResolveTargetIdentityName_DoesNotInvokeFallbackOnUnrelatedError(t *testing.T) {
	ctx := context.Background()
	stack := &schema.ConfigAndStacksInfo{}
	mgr := &stubAuthManager{defaultErr: errors.New("unrelated failure")}

	_, err := resolveTargetIdentityName(ctx, stack, mgr)
	assert.Error(t, err)
	assert.Zero(t, mgr.fallbackCalls,
		"fallback must only fire for no-auth-config errors, not arbitrary failures")
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

// TestComponentEnvSectionToList tests conversion from ComponentEnvSection map to env list.
func TestComponentEnvSectionToList(t *testing.T) {
	tests := []struct {
		name       string
		envSection map[string]any
		validate   func(t *testing.T, result []string)
	}{
		{
			name:       "nil map",
			envSection: nil,
			validate: func(t *testing.T, result []string) {
				assert.Empty(t, result)
			},
		},
		{
			name:       "empty map",
			envSection: map[string]any{},
			validate: func(t *testing.T, result []string) {
				assert.Empty(t, result)
			},
		},
		{
			name: "string values",
			envSection: map[string]any{
				"STRING_VAR": "value",
				"ANOTHER":    "test",
			},
			validate: func(t *testing.T, result []string) {
				assert.Len(t, result, 2)
				assert.Contains(t, result, "STRING_VAR=value")
				assert.Contains(t, result, "ANOTHER=test")
			},
		},
		{
			name: "numeric values",
			envSection: map[string]any{
				"INT_VAR":   123,
				"FLOAT_VAR": 45.67,
			},
			validate: func(t *testing.T, result []string) {
				assert.Len(t, result, 2)
				assert.Contains(t, result, "INT_VAR=123")
				assert.Contains(t, result, "FLOAT_VAR=45.67")
			},
		},
		{
			name: "boolean values",
			envSection: map[string]any{
				"BOOL_TRUE":  true,
				"BOOL_FALSE": false,
			},
			validate: func(t *testing.T, result []string) {
				assert.Len(t, result, 2)
				assert.Contains(t, result, "BOOL_TRUE=true")
				assert.Contains(t, result, "BOOL_FALSE=false")
			},
		},
		{
			name: "null values are excluded",
			envSection: map[string]any{
				"VALID":   "value",
				"NULL":    nil,
				"ALSO_OK": "test",
			},
			validate: func(t *testing.T, result []string) {
				// nil values should be excluded.
				assert.Len(t, result, 2)
				assert.Contains(t, result, "VALID=value")
				assert.Contains(t, result, "ALSO_OK=test")
				// Verify NULL is not present.
				for _, envVar := range result {
					assert.NotContains(t, envVar, "NULL=")
				}
			},
		},
		{
			name: "mixed types",
			envSection: map[string]any{
				"STRING": "text",
				"NUM":    42,
				"BOOL":   true,
				"NIL":    nil,
			},
			validate: func(t *testing.T, result []string) {
				assert.Len(t, result, 3)
				assert.Contains(t, result, "STRING=text")
				assert.Contains(t, result, "NUM=42")
				assert.Contains(t, result, "BOOL=true")
			},
		},
		{
			name: "empty string value",
			envSection: map[string]any{
				"EMPTY": "",
			},
			validate: func(t *testing.T, result []string) {
				assert.Len(t, result, 1)
				assert.Contains(t, result, "EMPTY=")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := componentEnvSectionToList(tt.envSection)
			tt.validate(t, result)
		})
	}
}
