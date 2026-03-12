package auth

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// trackingAuthManager extends stubAuthManager with call tracking for needs tests.
type trackingAuthManager struct {
	stubAuthManager
	authenticatedIdentities []string
	failIdentities          map[string]error
}

func (t *trackingAuthManager) Authenticate(ctx context.Context, identityName string) (*types.WhoamiInfo, error) {
	t.authenticatedIdentities = append(t.authenticatedIdentities, identityName)
	if err, ok := t.failIdentities[identityName]; ok {
		return nil, err
	}
	return t.whoami, nil
}

func newStackWithNeeds(needs []string) *schema.ConfigAndStacksInfo {
	authSection := schema.AtmosSectionMapType{
		"providers": map[string]any{
			"github-oidc": map[string]any{
				"kind": "github/oidc",
			},
		},
		"identities": map[string]any{
			"core-network": map[string]any{
				"kind": "aws/assume-role",
			},
			"plat-prod": map[string]any{
				"kind": "aws/assume-role",
			},
		},
	}
	if needs != nil {
		needsAny := make([]any, len(needs))
		for i, n := range needs {
			needsAny[i] = n
		}
		authSection["needs"] = needsAny
	}
	return &schema.ConfigAndStacksInfo{
		ComponentAuthSection: authSection,
	}
}

func TestResolveTargetIdentityName_NeedsList(t *testing.T) {
	stack := newStackWithNeeds([]string{"core-network", "plat-prod"})
	mgr := &stubAuthManager{defaultIdentity: "some-default"}

	name, err := resolveTargetIdentityName(stack, mgr)
	assert.NoError(t, err)
	assert.Equal(t, "some-default", name, "default identity should be primary even when needs is set")
}

func TestResolveTargetIdentityName_NeedsFallbackWhenNoDefault(t *testing.T) {
	stack := newStackWithNeeds([]string{"core-network", "plat-prod"})
	mgr := &stubAuthManager{defaultIdentity: ""}

	name, err := resolveTargetIdentityName(stack, mgr)
	assert.NoError(t, err)
	assert.Equal(t, "core-network", name, "first needs entry should be primary when no default identity exists")
}

func TestResolveTargetIdentityName_CliOverridesNeeds(t *testing.T) {
	stack := newStackWithNeeds([]string{"core-network", "plat-prod"})
	stack.Identity = "override-identity"
	mgr := &stubAuthManager{defaultIdentity: "some-default"}

	name, err := resolveTargetIdentityName(stack, mgr)
	assert.NoError(t, err)
	assert.Equal(t, "override-identity", name, "CLI --identity flag should take precedence over needs")
}

func TestResolveTargetIdentityName_NoNeeds(t *testing.T) {
	stack := newStackWithNeeds(nil)
	mgr := &stubAuthManager{defaultIdentity: "fallback-default"}

	name, err := resolveTargetIdentityName(stack, mgr)
	assert.NoError(t, err)
	assert.Equal(t, "fallback-default", name, "should fall back to default identity when needs is not set")
}

func TestAuthenticateAdditionalIdentities_Success(t *testing.T) {
	stack := newStackWithNeeds([]string{"primary", "secondary", "tertiary"})
	mgr := &trackingAuthManager{
		stubAuthManager: stubAuthManager{
			whoami: &types.WhoamiInfo{Provider: "p", Identity: "i"},
		},
	}

	authenticateAdditionalIdentities(context.Background(), mgr, "primary", stack)

	assert.Equal(t, []string{"secondary", "tertiary"}, mgr.authenticatedIdentities,
		"should authenticate all non-primary identities from needs")
}

func TestAuthenticateAdditionalIdentities_SkipsPrimary(t *testing.T) {
	stack := newStackWithNeeds([]string{"primary", "secondary"})
	mgr := &trackingAuthManager{
		stubAuthManager: stubAuthManager{
			whoami: &types.WhoamiInfo{Provider: "p", Identity: "i"},
		},
	}

	authenticateAdditionalIdentities(context.Background(), mgr, "primary", stack)

	for _, id := range mgr.authenticatedIdentities {
		assert.NotEqual(t, "primary", id, "primary identity should not be re-authenticated")
	}
}

func TestAuthenticateAdditionalIdentities_NonFatal(t *testing.T) {
	stack := newStackWithNeeds([]string{"primary", "fail-id", "success-id"})
	mgr := &trackingAuthManager{
		stubAuthManager: stubAuthManager{
			whoami: &types.WhoamiInfo{Provider: "p", Identity: "i"},
		},
		failIdentities: map[string]error{
			"fail-id": fmt.Errorf("simulated auth failure"),
		},
	}

	// Should not panic or return error — failures are non-fatal.
	authenticateAdditionalIdentities(context.Background(), mgr, "primary", stack)

	// Both identities should have been attempted despite the failure.
	assert.Contains(t, mgr.authenticatedIdentities, "fail-id",
		"failed identity should still be attempted")
	assert.Contains(t, mgr.authenticatedIdentities, "success-id",
		"subsequent identities should be attempted after a failure")
}

func TestAuthenticateAdditionalIdentities_EmptyNeeds(t *testing.T) {
	stack := newStackWithNeeds(nil)
	mgr := &trackingAuthManager{
		stubAuthManager: stubAuthManager{
			whoami: &types.WhoamiInfo{Provider: "p", Identity: "i"},
		},
	}

	authenticateAdditionalIdentities(context.Background(), mgr, "primary", stack)

	assert.Empty(t, mgr.authenticatedIdentities, "no identities should be authenticated when needs is empty")
}
