package auth

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// trackingAuthManager extends stubAuthManager with call tracking for required identity tests.
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

func TestResolveTargetIdentityName_DefaultWinsOverRequired(t *testing.T) {
	stack := &schema.ConfigAndStacksInfo{}
	mgr := &stubAuthManager{defaultIdentity: "some-default"}

	name, err := resolveTargetIdentityName(stack, mgr)
	assert.NoError(t, err)
	assert.Equal(t, "some-default", name, "default identity should be primary even when required identities exist")
}

func TestResolveTargetIdentityName_NoDefaultErrorsEvenWithRequired(t *testing.T) {
	stack := &schema.ConfigAndStacksInfo{}
	mgr := &stubAuthManager{
		defaultIdentity: "",
		identities: map[string]schema.Identity{
			"core-network": {Kind: "aws/assume-role", Required: true},
			"plat-prod":    {Kind: "aws/assume-role", Required: true},
		},
	}

	_, err := resolveTargetIdentityName(stack, mgr)
	assert.ErrorIs(t, err, errUtils.ErrNoDefaultIdentity, "should error when no default, even if required identities exist")
}

func TestResolveTargetIdentityName_CliOverridesDefault(t *testing.T) {
	stack := &schema.ConfigAndStacksInfo{Identity: "override-identity"}
	mgr := &stubAuthManager{defaultIdentity: "some-default"}

	name, err := resolveTargetIdentityName(stack, mgr)
	assert.NoError(t, err)
	assert.Equal(t, "override-identity", name, "CLI --identity flag should take precedence")
}

func TestResolveTargetIdentityName_NoDefaultNoRequired(t *testing.T) {
	stack := &schema.ConfigAndStacksInfo{}
	mgr := &stubAuthManager{defaultIdentity: ""}

	_, err := resolveTargetIdentityName(stack, mgr)
	assert.ErrorIs(t, err, errUtils.ErrNoDefaultIdentity, "should error when no default and no required")
}

func TestAuthenticateAdditionalIdentities_RequiredSuccess(t *testing.T) {
	mgr := &trackingAuthManager{
		stubAuthManager: stubAuthManager{
			whoami: &types.WhoamiInfo{Provider: "p", Identity: "i"},
			identities: map[string]schema.Identity{
				"primary":   {Kind: "aws/assume-role", Required: true},
				"secondary": {Kind: "aws/assume-role", Required: true},
				"tertiary":  {Kind: "aws/assume-role", Required: true},
			},
		},
	}

	authenticateAdditionalIdentities(context.Background(), mgr, "primary")

	assert.Contains(t, mgr.authenticatedIdentities, "secondary",
		"should authenticate required non-primary identities")
	assert.Contains(t, mgr.authenticatedIdentities, "tertiary",
		"should authenticate required non-primary identities")
	assert.NotContains(t, mgr.authenticatedIdentities, "primary",
		"primary identity should not be re-authenticated")
}

func TestAuthenticateAdditionalIdentities_SkipsNonRequired(t *testing.T) {
	mgr := &trackingAuthManager{
		stubAuthManager: stubAuthManager{
			whoami: &types.WhoamiInfo{Provider: "p", Identity: "i"},
			identities: map[string]schema.Identity{
				"primary":       {Kind: "aws/assume-role", Required: true},
				"optional":      {Kind: "aws/assume-role", Required: false},
				"also-required": {Kind: "aws/assume-role", Required: true},
			},
		},
	}

	authenticateAdditionalIdentities(context.Background(), mgr, "primary")

	assert.Contains(t, mgr.authenticatedIdentities, "also-required",
		"should authenticate required identities")
	assert.NotContains(t, mgr.authenticatedIdentities, "optional",
		"should skip non-required identities")
}

func TestAuthenticateAdditionalIdentities_SkipsPrimary(t *testing.T) {
	mgr := &trackingAuthManager{
		stubAuthManager: stubAuthManager{
			whoami: &types.WhoamiInfo{Provider: "p", Identity: "i"},
			identities: map[string]schema.Identity{
				"primary":   {Kind: "aws/assume-role", Required: true},
				"secondary": {Kind: "aws/assume-role", Required: true},
			},
		},
	}

	authenticateAdditionalIdentities(context.Background(), mgr, "primary")

	for _, id := range mgr.authenticatedIdentities {
		assert.NotEqual(t, "primary", id, "primary identity should not be re-authenticated")
	}
}

func TestAuthenticateAdditionalIdentities_NonFatal(t *testing.T) {
	mgr := &trackingAuthManager{
		stubAuthManager: stubAuthManager{
			whoami: &types.WhoamiInfo{Provider: "p", Identity: "i"},
			identities: map[string]schema.Identity{
				"primary":    {Kind: "aws/assume-role", Required: true},
				"fail-id":    {Kind: "aws/assume-role", Required: true},
				"success-id": {Kind: "aws/assume-role", Required: true},
			},
		},
		failIdentities: map[string]error{
			"fail-id": fmt.Errorf("simulated auth failure"),
		},
	}

	// Should not panic or return error — failures are non-fatal.
	authenticateAdditionalIdentities(context.Background(), mgr, "primary")

	// Both identities should have been attempted despite the failure.
	assert.Contains(t, mgr.authenticatedIdentities, "fail-id",
		"failed identity should still be attempted")
	assert.Contains(t, mgr.authenticatedIdentities, "success-id",
		"subsequent identities should be attempted after a failure")
}

func TestAuthenticateAdditionalIdentities_NoRequired(t *testing.T) {
	mgr := &trackingAuthManager{
		stubAuthManager: stubAuthManager{
			whoami: &types.WhoamiInfo{Provider: "p", Identity: "i"},
			identities: map[string]schema.Identity{
				"identity-a": {Kind: "aws/assume-role"},
				"identity-b": {Kind: "aws/assume-role"},
			},
		},
	}

	authenticateAdditionalIdentities(context.Background(), mgr, "primary")

	assert.Empty(t, mgr.authenticatedIdentities, "no identities should be authenticated when none are required")
}

func TestAuthenticateAdditionalIdentities_EmptyIdentities(t *testing.T) {
	mgr := &trackingAuthManager{
		stubAuthManager: stubAuthManager{
			whoami: &types.WhoamiInfo{Provider: "p", Identity: "i"},
		},
	}

	authenticateAdditionalIdentities(context.Background(), mgr, "primary")

	assert.Empty(t, mgr.authenticatedIdentities, "no identities should be authenticated when identities map is empty")
}
