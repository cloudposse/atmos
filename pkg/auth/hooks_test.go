package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stubAuthManager implements types.AuthManager for focused unit tests.
type stubAuthManager struct {
	defaultIdentity string
	defaultErr      error
	whoami          *types.WhoamiInfo
}

func (s *stubAuthManager) Authenticate(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	return s.whoami, nil
}

func (s *stubAuthManager) Whoami(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
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
	return make(map[string]string), nil
}

func TestGetConfigLogLevels(t *testing.T) {
	// Nil config falls back to Info.
	atmos, auth := getConfigLogLevels(nil)
	assert.Equal(t, "info", atmos.String())
	assert.Equal(t, "info", auth.String())

	cfg := &schema.AtmosConfiguration{}
	atmos, auth = getConfigLogLevels(cfg)
	assert.Equal(t, "info", atmos.String())
	assert.Equal(t, "info", auth.String())

	cfg.Logs.Level = "Debug"
	atmos, auth = getConfigLogLevels(cfg)
	assert.Equal(t, "debug", atmos.String())
	assert.Equal(t, "debug", auth.String())

	cfg.Auth.Logs.Level = "Error"
	atmos, auth = getConfigLogLevels(cfg)
	assert.Equal(t, "debug", atmos.String()) // unchanged from cfg.Logs
	assert.Equal(t, "error", auth.String())  // overridden by cfg.Auth.Logs
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
