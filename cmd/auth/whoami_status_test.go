package auth

import (
	"context"
	"encoding/json"
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

// TestReportWhoami_AmbientIdentityExitsZero reproduces the 1.235.0 upgrade regression where
// `atmos auth whoami` (and `-o json`) exited 1 for an ambient identity. Such an identity never
// returns credentials by design, so there is nothing to validate: whoami must succeed and say the
// credentials are not managed by Atmos, instead of reporting them as expired or invalid.
func TestReportWhoami_AmbientIdentityExitsZero(t *testing.T) {
	ambient := func() *authTypes.WhoamiInfo {
		return &authTypes.WhoamiInfo{Identity: "amb", Provider: "ambient", Ambient: true}
	}

	t.Run("human", func(t *testing.T) {
		_, stderr := captureWhoamiStreams(t)
		require.NoError(t, reportWhoami(context.Background(), ambient(), ""))
		text := plainWhoamiText(stderr)
		assert.Contains(t, text, "not managed by Atmos")
		assert.NotContains(t, text, "expired or invalid")
		assert.NotContains(t, text, "atmos auth login")
	})

	t.Run("json", func(t *testing.T) {
		stdout, stderr := captureWhoamiStreams(t)
		require.NoError(t, reportWhoami(context.Background(), ambient(), "json"))
		assert.NotContains(t, plainWhoamiText(stderr), "expired or invalid")
		var doc map[string]any
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
		assert.Equal(t, true, doc["ambient"])
		assert.Equal(t, "amb", doc["identity"])
	})
}

// TestReportWhoami_MissingManagedCredentialsStillFail is the negative path: an identity that is
// not ambient but has no credentials still exits 1, in both formats.
func TestReportWhoami_MissingManagedCredentialsStillFail(t *testing.T) {
	for _, format := range []string{"", "json"} {
		t.Run("format="+format, func(t *testing.T) {
			captureWhoamiStreams(t)
			err := reportWhoami(context.Background(), &authTypes.WhoamiInfo{Identity: "dev", Provider: "sso"}, format)
			require.ErrorIs(t, err, errUtils.ErrExpiredCredentials)
			assert.Equal(t, 1, errUtils.GetExitCode(err))
		})
	}
}
