package credentialprocess

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

// Compile-time guard so a field rename in AWSCredentials fails the build here.
var _ = types.AWSCredentials{AccessKeyID: "", SecretAccessKey: "", SessionToken: "", Expiration: ""}

const testIdentity = "app-sandbox-1"

var fixedNow = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

func fixedClock() time.Time { return fixedNow }

func awsCreds(expiration string) *types.AWSCredentials {
	return &types.AWSCredentials{
		AccessKeyID:     "ASIAEXAMPLEKEY",
		SecretAccessKey: "secret/example",
		SessionToken:    "token-example",
		Expiration:      expiration,
	}
}

func whoami(creds types.ICredentials) *types.WhoamiInfo {
	return &types.WhoamiInfo{Identity: testIdentity, Credentials: creds}
}

func TestProduce_CachedValid_NoAuthenticate(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := types.NewMockAuthManager(ctrl)
	exp := fixedNow.Add(time.Hour).Format(time.RFC3339)
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(whoami(awsCreds(exp)), nil)
	// Authenticate is intentionally not expected: gomock fails the test if it is called.

	out, err := Produce(context.Background(), mgr, testIdentity, WithClock(fixedClock))
	require.NoError(t, err)
	assert.Equal(t,
		`{"Version":1,"AccessKeyId":"ASIAEXAMPLEKEY","SecretAccessKey":"secret/example","SessionToken":"token-example","Expiration":"2026-10-02T13:00:00Z"}`,
		string(out))
}

func TestProduce_CachedWithinMinValidity_Authenticates(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := types.NewMockAuthManager(ctrl)
	soon := fixedNow.Add(5 * time.Minute).Format(time.RFC3339)
	fresh := fixedNow.Add(time.Hour).Format(time.RFC3339)
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(whoami(awsCreds(soon)), nil)
	mgr.EXPECT().Authenticate(gomock.Any(), testIdentity).Return(whoami(awsCreds(fresh)), nil)

	out, err := Produce(context.Background(), mgr, testIdentity, WithClock(fixedClock))
	require.NoError(t, err)
	assert.Contains(t, string(out), `"Expiration":"2026-10-02T13:00:00Z"`)
}

func TestProduce_CustomMinValidity(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := types.NewMockAuthManager(ctrl)
	exp := fixedNow.Add(5 * time.Minute).Format(time.RFC3339)
	// Five minutes of remaining validity satisfies a one minute minimum.
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(whoami(awsCreds(exp)), nil)

	out, err := Produce(context.Background(), mgr, testIdentity, WithClock(fixedClock), WithMinValidity(time.Minute))
	require.NoError(t, err)
	assert.Contains(t, string(out), `"Expiration":"2026-10-02T12:05:00Z"`)
}

func TestProduce_CachedError_Authenticates(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := types.NewMockAuthManager(ctrl)
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(nil, errUtils.ErrNoCredentialsFound)
	mgr.EXPECT().Authenticate(gomock.Any(), testIdentity).Return(whoami(awsCreds("")), nil)

	out, err := Produce(context.Background(), mgr, testIdentity, WithClock(fixedClock))
	require.NoError(t, err)
	assert.Equal(t,
		`{"Version":1,"AccessKeyId":"ASIAEXAMPLEKEY","SecretAccessKey":"secret/example","SessionToken":"token-example"}`,
		string(out))
}

func TestProduce_LongLivedCached_Authenticates(t *testing.T) {
	// Cached credentials without an expiration are long-lived keys (for example an IAM user's
	// keyring entry). They must never be printed directly; authenticating exchanges them for a
	// session instead.
	ctrl := gomock.NewController(t)
	mgr := types.NewMockAuthManager(ctrl)
	longLived := &types.AWSCredentials{AccessKeyID: "AKIAEXAMPLEKEY", SecretAccessKey: "secret/example"}
	session := &types.AWSCredentials{
		AccessKeyID:     "ASIASESSIONKEY",
		SecretAccessKey: "session/secret",
		SessionToken:    "session-token",
		Expiration:      fixedClock().Add(time.Hour).UTC().Format(time.RFC3339),
	}
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(whoami(longLived), nil)
	mgr.EXPECT().Authenticate(gomock.Any(), testIdentity).Return(whoami(session), nil)

	out, err := Produce(context.Background(), mgr, testIdentity, WithClock(fixedClock))
	require.NoError(t, err)
	assert.NotContains(t, string(out), "AKIAEXAMPLEKEY")
	assert.Contains(t, string(out), `"AccessKeyId":"ASIASESSIONKEY"`)
}

func TestProduce_CachedWithoutCredentials_Authenticates(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := types.NewMockAuthManager(ctrl)
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(&types.WhoamiInfo{}, nil)
	mgr.EXPECT().Authenticate(gomock.Any(), testIdentity).Return(whoami(awsCreds("")), nil)

	_, err := Produce(context.Background(), mgr, testIdentity, WithClock(fixedClock))
	require.NoError(t, err)
}

func TestProduce_CachedUnparseableExpiration_Authenticates(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := types.NewMockAuthManager(ctrl)
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(whoami(awsCreds("not-a-time")), nil)
	mgr.EXPECT().Authenticate(gomock.Any(), testIdentity).Return(whoami(awsCreds("")), nil)

	_, err := Produce(context.Background(), mgr, testIdentity, WithClock(fixedClock))
	require.NoError(t, err)
}

func TestProduce_NonAWSCached_ErrIdentityNotAWS(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := types.NewMockAuthManager(ctrl)
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(whoami(&types.AzureCredentials{}), nil)

	_, err := Produce(context.Background(), mgr, testIdentity)
	require.ErrorIs(t, err, errUtils.ErrIdentityNotAWS)
	assert.Contains(t, err.Error(), testIdentity)
}

func TestProduce_NonAWSAuthenticated_ErrIdentityNotAWS(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := types.NewMockAuthManager(ctrl)
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(nil, errUtils.ErrNoCredentialsFound)
	mgr.EXPECT().Authenticate(gomock.Any(), testIdentity).Return(whoami(&types.AzureCredentials{}), nil)

	_, err := Produce(context.Background(), mgr, testIdentity)
	require.ErrorIs(t, err, errUtils.ErrIdentityNotAWS)
	assert.Contains(t, err.Error(), testIdentity)
}

func TestProduce_AuthenticateError_Propagates(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := types.NewMockAuthManager(ctrl)
	cause := errors.New("sso session expired")
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(nil, errUtils.ErrNoCredentialsFound)
	mgr.EXPECT().Authenticate(gomock.Any(), testIdentity).Return(nil, cause)

	_, err := Produce(context.Background(), mgr, testIdentity)
	require.ErrorIs(t, err, errUtils.ErrIdentityAuthFailed)
	require.ErrorIs(t, err, cause)
}

func TestProduce_AuthenticateReturnsNoCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := types.NewMockAuthManager(ctrl)
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(nil, errUtils.ErrNoCredentialsFound)
	mgr.EXPECT().Authenticate(gomock.Any(), testIdentity).Return(&types.WhoamiInfo{}, nil)

	_, err := Produce(context.Background(), mgr, testIdentity)
	require.ErrorIs(t, err, errUtils.ErrIdentityCredentialsNone)
}

func TestProduce_SkipsIntegrations(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := types.NewMockAuthManager(ctrl)
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(nil, errUtils.ErrNoCredentialsFound)
	mgr.EXPECT().Authenticate(gomock.Any(), testIdentity).DoAndReturn(
		func(ctx context.Context, _ string) (*types.WhoamiInfo, error) {
			assert.True(t, auth.IntegrationsSkipped(ctx), "integrations must be skipped while producing credentials")
			return whoami(awsCreds("")), nil
		},
	)

	_, err := Produce(context.Background(), mgr, testIdentity)
	require.NoError(t, err)
}

func TestProduce_MissingKeys_ErrIdentityNotAWS(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := types.NewMockAuthManager(ctrl)
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(whoami(&types.AWSCredentials{Expiration: fixedClock().Add(time.Hour).UTC().Format(time.RFC3339)}), nil)

	_, err := Produce(context.Background(), mgr, testIdentity, WithClock(fixedClock))
	require.ErrorIs(t, err, errUtils.ErrIdentityNotAWS)
}

func TestResolveIdentity(t *testing.T) {
	selectErr := errUtils.ErrIdentitySelectionRequiresTTY

	tests := []struct {
		name      string
		input     string
		setup     func(m *types.MockAuthManager)
		want      string
		wantErrIs error
	}{
		{name: "explicit name", input: "dev", setup: func(*types.MockAuthManager) {}, want: "dev"},
		{
			name:  "empty uses default",
			input: "",
			setup: func(m *types.MockAuthManager) { m.EXPECT().GetDefaultIdentity(false).Return("default-id", nil) },
			want:  "default-id",
		},
		{
			name:      "empty without default",
			input:     "",
			setup:     func(m *types.MockAuthManager) { m.EXPECT().GetDefaultIdentity(false).Return("", errors.New("none")) },
			wantErrIs: errUtils.ErrNoDefaultIdentity,
		},
		{
			name:  "select sentinel opens selector",
			input: cfg.IdentityFlagSelectValue,
			setup: func(m *types.MockAuthManager) { m.EXPECT().GetDefaultIdentity(true).Return("picked", nil) },
			want:  "picked",
		},
		{
			name:      "select sentinel without TTY",
			input:     cfg.IdentityFlagSelectValue,
			setup:     func(m *types.MockAuthManager) { m.EXPECT().GetDefaultIdentity(true).Return("", selectErr) },
			wantErrIs: selectErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := types.NewMockAuthManager(gomock.NewController(t))
			tt.setup(mgr)

			got, err := ResolveIdentity(mgr, tt.input)
			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
