package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

// whoamiWithCredentials builds a WhoamiInfo whose credentials report the given
// expiry and do not implement validation, so validity falls back to IsExpired.
func whoamiWithCredentials(t *testing.T, expired bool) *authTypes.WhoamiInfo {
	t.Helper()
	creds := authTypes.NewMockICredentials(gomock.NewController(t))
	creds.EXPECT().Validate(gomock.Any()).Return(nil, errUtils.ErrNotImplemented).AnyTimes()
	creds.EXPECT().IsExpired().Return(expired).AnyTimes()
	expiration := time.Now().Add(time.Hour)
	if expired {
		expiration = time.Now().Add(-time.Hour)
	}
	creds.EXPECT().GetExpiration().Return(&expiration, nil).AnyTimes()
	return &authTypes.WhoamiInfo{Identity: "dev", Provider: "sso", Credentials: creds, Expiration: &expiration}
}

// TestReportWhoami_ExitStatusReflectsCredentialValidity verifies that
// `atmos auth whoami` fails (non-zero exit) when credentials are present but
// expired or invalid, in both output formats, and succeeds when they are valid.
func TestReportWhoami_ExitStatusReflectsCredentialValidity(t *testing.T) {
	for _, format := range []string{"", "json"} {
		t.Run("format="+format+"/expired", func(t *testing.T) {
			err := reportWhoami(context.Background(), whoamiWithCredentials(t, true), format)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrExpiredCredentials)
			assert.Equal(t, 1, errUtils.GetExitCode(err))
			var exitErr errUtils.ExitCodeError
			require.True(t, errors.As(err, &exitErr))
			assert.True(t, exitErr.Silent, "the status is already reported, so no extra error box is rendered")
		})
		t.Run("format="+format+"/valid", func(t *testing.T) {
			assert.NoError(t, reportWhoami(context.Background(), whoamiWithCredentials(t, false), format))
		})
	}
}
