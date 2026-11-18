package list

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
)

// Compile-time guard so a WhoamiInfo field rename fails the build here.
var _ = authTypes.WhoamiInfo{Expiration: nil}

const statusTestNow = "2026-01-01T12:00:00Z"

// cachedCredentialsManager returns a mock whose cached credentials for "dev"
// expire `remaining` after statusTestNow; a nil remaining means no expiration.
func cachedCredentialsManager(t *testing.T, remaining *time.Duration, lookupErr error) *authTypes.MockAuthManager {
	t.Helper()
	now, err := time.Parse(time.RFC3339, statusTestNow)
	require.NoError(t, err)
	manager := authTypes.NewMockAuthManager(gomock.NewController(t))
	call := manager.EXPECT().GetCachedCredentials(gomock.Any(), "dev")
	switch {
	case lookupErr != nil:
		call.Return(nil, lookupErr).AnyTimes()
	case remaining == nil:
		call.Return(&authTypes.WhoamiInfo{Identity: "dev"}, nil).AnyTimes()
	default:
		expiration := now.Add(*remaining)
		call.Return(&authTypes.WhoamiInfo{Identity: "dev", Expiration: &expiration}, nil).AnyTimes()
	}
	return manager
}

func durationPtr(d time.Duration) *time.Duration { return &d }

// TestIdentityStatusAndExpirationAgree pins the status thresholds shared by the
// status dot and the expiration column, including both boundary instants.
func TestIdentityStatusAndExpirationAgree(t *testing.T) {
	t.Setenv("ATMOS_TEST_NOW", statusTestNow)

	for _, tt := range []struct {
		name         string
		remaining    *time.Duration
		lookupErr    error
		wantStatus   authStatus
		wantDuration string
	}{
		{name: "no cached credentials", lookupErr: errors.New("not cached"), wantStatus: authStatusUnknown},
		{name: "credentials without expiration are valid", wantStatus: authStatusValid},
		{name: "expired one second ago", remaining: durationPtr(-time.Second), wantStatus: authStatusExpired, wantDuration: "expired"},
		{name: "expiring exactly now counts as expired", remaining: durationPtr(0), wantStatus: authStatusExpired, wantDuration: "expired"},
		{name: "one second left is expiring", remaining: durationPtr(time.Second), wantStatus: authStatusExpiring, wantDuration: "1s"},
		{name: "exactly at the threshold is expiring", remaining: durationPtr(expiringThreshold), wantStatus: authStatusExpiring, wantDuration: "15m"},
		{name: "just past the threshold is valid", remaining: durationPtr(expiringThreshold + time.Second), wantStatus: authStatusValid, wantDuration: "15m"},
		{name: "hours and minutes", remaining: durationPtr(2*time.Hour + 5*time.Minute), wantStatus: authStatusValid, wantDuration: "2h5m"},
		{name: "whole hours drop zero minutes", remaining: durationPtr(3 * time.Hour), wantStatus: authStatusValid, wantDuration: "3h"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			manager := cachedCredentialsManager(t, tt.remaining, tt.lookupErr)

			assert.Equal(t, tt.wantStatus, getIdentityAuthStatus(manager, "dev"))

			duration, status := getExpirationInfo(manager, "dev")
			assert.Equal(t, tt.wantStatus, status, "the expiration column must agree with the status dot")
			assert.Equal(t, tt.wantDuration, duration)

			expiration := resolveIdentityExpiration(manager, "dev")
			if tt.wantDuration == "" {
				assert.Equal(t, emptyMarker, expiration)
			} else {
				assert.Contains(t, expiration, tt.wantDuration)
			}
		})
	}
}

func TestIdentityStatusWithoutManager(t *testing.T) {
	assert.Equal(t, authStatusUnknown, getIdentityAuthStatus(nil, "dev"))
	duration, status := getExpirationInfo(nil, "dev")
	assert.Empty(t, duration)
	assert.Equal(t, authStatusUnknown, status)
	assert.Equal(t, emptyMarker, resolveIdentityExpiration(nil, "dev"))
}

func TestIdentityStatusNilWhoamiIsUnknown(t *testing.T) {
	manager := authTypes.NewMockAuthManager(gomock.NewController(t))
	manager.EXPECT().GetCachedCredentials(gomock.Any(), "dev").Return(nil, nil).AnyTimes()
	assert.Equal(t, authStatusUnknown, getIdentityAuthStatus(manager, "dev"))
	duration, status := getExpirationInfo(manager, "dev")
	assert.Empty(t, duration)
	assert.Equal(t, authStatusUnknown, status)
}

func TestFormatDurationRounding(t *testing.T) {
	for _, tt := range []struct {
		in   time.Duration
		want string
	}{
		{-time.Minute, "expired"},
		{0, "expired"},
		{499 * time.Millisecond, "0s"},
		{30 * time.Second, "30s"},
		{59*time.Second + 600*time.Millisecond, "1m"},
		{45 * time.Minute, "45m"},
		{59*time.Minute + 59*time.Second + 600*time.Millisecond, "1h"},
		{90 * time.Minute, "1h30m"},
		{26 * time.Hour, "26h"},
	} {
		assert.Equal(t, tt.want, formatDuration(tt.in), "formatDuration(%s)", tt.in)
	}
}

func TestFormatExpirationWithColorKeepsText(t *testing.T) {
	assert.Empty(t, formatExpirationWithColor("", authStatusValid))
	assert.Equal(t, "5m", formatExpirationWithColor("5m", authStatusUnknown), "unknown status is left unstyled")
	for _, status := range []authStatus{authStatusValid, authStatusExpiring, authStatusExpired} {
		assert.Contains(t, formatExpirationWithColor("5m", status), "5m")
	}
}
