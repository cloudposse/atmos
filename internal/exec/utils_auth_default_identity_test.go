package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

// TestResolveDefaultIdentity keeps a missing default non-fatal for an implicit request while
// conflicting defaults and a failed bare --identity selection are errors.
func TestResolveDefaultIdentity(t *testing.T) {
	for _, tc := range []struct {
		name      string
		requested string
		lookup    string
		lookupErr error
		want      string
		wantErr   error
	}{
		{name: "single default", lookup: "dev", want: "dev"},
		{name: "no default is implicit", lookupErr: errUtils.ErrNoDefaultIdentity},
		{name: "no default fails a bare selection", requested: cfg.IdentityFlagSelectValue, lookupErr: errUtils.ErrNoDefaultIdentity, wantErr: errUtils.ErrAuthenticationFailed},
		{name: "conflicting defaults are never ignored", lookupErr: errUtils.ErrMultipleDefaultIdentities, wantErr: errUtils.ErrMultipleDefaultIdentities},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := authtypes.NewMockAuthManager(gomock.NewController(t))
			manager.EXPECT().GetDefaultIdentity(false).Return(tc.lookup, tc.lookupErr)

			got, err := resolveDefaultIdentity(manager, tc.requested)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.want != "" {
				assert.Equal(t, tc.want, got)
			} else {
				assert.Equal(t, tc.requested, got)
			}
		})
	}
}
