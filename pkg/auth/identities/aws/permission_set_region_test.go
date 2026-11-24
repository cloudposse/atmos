package aws

// Tests that a permission set's principal.region, not the Identity Center
// region, is stamped onto its credentials (issue #3720, finding 3.9).

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newRegionalPermissionSet builds a portal-backed permission set with principal.region set.
func newRegionalPermissionSet(t *testing.T, endpoint, region string) *permissionSetIdentity {
	t.Helper()

	principal := map[string]any{
		"name":    "AdminAccess",
		"account": map[string]any{"id": "222222222222"},
	}
	if region != "" {
		principal["region"] = region
	}
	identity, err := NewPermissionSetIdentity("prod-admin", &schema.Identity{
		Kind:      "aws/permission-set",
		Principal: principal,
		Spec:      map[string]any{"endpoint_url": endpoint},
	})
	require.NoError(t, err)
	ps, ok := identity.(*permissionSetIdentity)
	require.True(t, ok)
	return ps
}

func TestPermissionSetIdentity_Authenticate_UsesPrincipalRegion(t *testing.T) {
	tests := []struct {
		name            string
		principalRegion string
		withManager     bool
		managerRegion   string
		want            string
	}{
		{name: "principal.region overrides the Identity Center region", principalRegion: "eu-west-1", want: "eu-west-1"},
		{name: "without principal.region the Identity Center region is kept", want: "us-east-1"},
		{name: "region inherited through the manager's identity chain", withManager: true, managerRegion: "ap-south-1", want: "ap-south-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
			portal := &fakeSSOPortal{}
			srv := httptest.NewServer(portal.handler(t))
			t.Cleanup(srv.Close)

			ps := newRegionalPermissionSet(t, srv.URL, tt.principalRegion)
			if tt.withManager {
				manager := types.NewMockAuthManager(gomock.NewController(t))
				manager.EXPECT().ResolvePrincipalSetting("prod-admin", "region").Return(tt.managerRegion, true).AnyTimes()
				ps.manager = manager
			}

			// The Identity Center (SSO) region of the base credentials.
			ssoBase := &types.AWSCredentials{AccessKeyID: "sso-bearer-token", Region: "us-east-1"}
			creds, err := ps.Authenticate(context.Background(), ssoBase)
			require.NoError(t, err)

			awsCreds, ok := creds.(*types.AWSCredentials)
			require.True(t, ok)
			assert.Equal(t, tt.want, awsCreds.Region)
			assert.Equal(t, int32(1), portal.credsCalls.Load(), "the role credentials still come from the SSO portal")
		})
	}
}
