package auth

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/schema"
)

func installProductionAuthPolicy(t *testing.T, variant string) {
	t.Helper()
	t.Cleanup(guard.SaveHooks())
	bundle, err := policy.LoadDirectory(filepath.Join("..", "..", "policy-library", "auth-deny-production", variant))
	require.NoError(t, err)
	guard.SetEnabled(func(kind string) (bool, error) { return kind == "auth", nil })
	guard.SetEvaluator(func(ctx context.Context, kind string, raw json.RawMessage) error {
		decision, err := policy.NewEngine().Evaluate(ctx, &policy.Request{Kind: policy.InputKind(kind), Input: raw}, bundle.Rules)
		if err != nil {
			return err
		}
		return policy.EnforceDecision(decision)
	})
}

func TestAuthPolicyProductionChains(t *testing.T) {
	for _, variant := range []string{"", filepath.Join("variants", "starlark")} {
		t.Run(variant, func(t *testing.T) {
			installProductionAuthPolicy(t, variant)
			for _, tc := range []struct {
				name, provider, root, target string
			}{
				{"provider", "prod-sso", "base", "dev"},
				{"intermediate", "sso", "prod-admin", "dev"},
				{"target", "sso", "base", "prod"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					m := productionPolicyManager(t, tc.provider, tc.root, tc.target)
					_, err := m.Authenticate(context.Background(), tc.target)
					require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
					_, err = m.GetCachedCredentials(context.Background(), tc.target)
					require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
					// Even an already-authenticated process cache cannot bypass the rule.
					exp := time.Now().Add(time.Hour)
					key := m.chainCacheKey()
					processCredentialCache.Store(key, &processCachedCreds{credentials: &testCreds{exp: &exp}})
					t.Cleanup(func() { processCredentialCache.Delete(key) })
					_, err = m.authenticateChain(context.Background(), tc.target)
					require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
				})
			}
			m := productionPolicyManager(t, "prod-sso", "base", "dev")
			_, err := m.AuthenticateProvider(context.Background(), "PROD-SSO")
			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		})
	}
}

func TestAuthPolicyEnvironmentExports(t *testing.T) {
	installProductionAuthPolicy(t, "")
	m := productionPolicyManager(t, "prod-sso", "base", "dev")
	_, err := m.GetEnvironmentVariables("dev")
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	_, err = m.PrepareShellEnvironment(context.Background(), "dev", []string{"EXISTING=value"})
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
}

func TestAuthPolicyStandaloneIdentity(t *testing.T) {
	installProductionAuthPolicy(t, "")
	ctrl := gomock.NewController(t)
	m := &manager{
		config: &schema.AuthConfig{Identities: map[string]schema.Identity{
			"prod-helper": {Kind: "aws/credential-process"},
		}},
		identities:      map[string]types.Identity{"prod-helper": types.NewMockIdentity(ctrl)},
		credentialStore: types.NewMockCredentialStore(ctrl),
	}
	_, err := m.Authenticate(context.Background(), "prod-helper")
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	_, err = m.GetCachedCredentials(context.Background(), "prod-helper")
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
}

// Mocks have no credential-I/O expectations: any call before denial fails the test.
func productionPolicyManager(t *testing.T, provider, root, target string) *manager {
	t.Helper()
	ctrl := gomock.NewController(t)
	return &manager{
		config: &schema.AuthConfig{Identities: map[string]schema.Identity{
			root:   {Kind: "aws/assume-role", Via: &schema.IdentityVia{Provider: provider}},
			target: {Kind: "aws/assume-role", Via: &schema.IdentityVia{Identity: root}},
		}},
		providers: map[string]types.Provider{provider: types.NewMockProvider(ctrl)},
		identities: map[string]types.Identity{
			root: types.NewMockIdentity(ctrl), target: types.NewMockIdentity(ctrl),
		},
		credentialStore: types.NewMockCredentialStore(ctrl),
	}
}
