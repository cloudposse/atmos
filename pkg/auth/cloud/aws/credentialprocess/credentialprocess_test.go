package credentialprocess

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
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

func TestProduce_MissingKeys_ErrAWSCredentialsIncomplete(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := types.NewMockAuthManager(ctrl)
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(whoami(&types.AWSCredentials{Expiration: fixedClock().Add(time.Hour).UTC().Format(time.RFC3339)}), nil)

	_, err := Produce(context.Background(), mgr, testIdentity, WithClock(fixedClock))
	require.ErrorIs(t, err, errUtils.ErrAWSCredentialsIncomplete)
	// These are AWS credentials, just incomplete ones.
	assert.NotErrorIs(t, err, errUtils.ErrIdentityNotAWS)
}

func identitiesWithDefaults(defaults ...string) map[string]schema.Identity {
	identities := map[string]schema.Identity{"plain": {Kind: "aws/user"}}
	for _, name := range defaults {
		identities[name] = schema.Identity{Kind: "aws/user", Default: true}
	}
	return identities
}

func TestResolveIdentity(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		setup     func(m *types.MockAuthManager)
		want      string
		wantErrIs error
	}{
		{name: "explicit name", input: "dev", setup: func(*types.MockAuthManager) {}, want: "dev"},
		{
			name:  "empty uses the single default",
			input: "",
			setup: func(m *types.MockAuthManager) {
				m.EXPECT().GetIdentities().Return(identitiesWithDefaults("default-id"))
			},
			want: "default-id",
		},
		{
			name:  "empty without a default",
			input: "",
			setup: func(m *types.MockAuthManager) {
				m.EXPECT().GetIdentities().Return(identitiesWithDefaults())
			},
			wantErrIs: errUtils.ErrNoDefaultIdentity,
		},
		{
			name:  "empty with several defaults",
			input: "",
			setup: func(m *types.MockAuthManager) {
				m.EXPECT().GetIdentities().Return(identitiesWithDefaults("b-default", "a-default"))
			},
			wantErrIs: errUtils.ErrMultipleDefaultIdentities,
		},
		{
			name:      "bare --identity never opens the selector",
			input:     cfg.IdentityFlagSelectValue,
			setup:     func(*types.MockAuthManager) {}, // GetDefaultIdentity is not expected: gomock fails if it is called.
			wantErrIs: errUtils.ErrCredentialProcessIdentityRequired,
		},
		{
			name:      "false-like values that were not normalized by the caller are rejected too",
			input:     "false",
			setup:     func(*types.MockAuthManager) {},
			wantErrIs: errUtils.ErrCredentialProcessIdentityRequired,
		},
		{
			name:      "--identity=false is rejected instead of leaking the sentinel",
			input:     cfg.IdentityFlagDisabledValue,
			setup:     func(*types.MockAuthManager) {},
			wantErrIs: errUtils.ErrCredentialProcessIdentityRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := types.NewMockAuthManager(gomock.NewController(t))
			tt.setup(mgr)

			got, err := ResolveIdentity(mgr, tt.input)
			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
				assert.Empty(t, got)
				// Every failure tells the user how to fix it, as a hint rather than a duplicated sentence.
				hints := strings.Join(errUtils.AllHints(err), "\n")
				assert.Contains(t, hints, "--identity=<name>")
				assert.Contains(t, hints, "atmos auth list")
				// The internal sentinels must never reach the user.
				assert.NotContains(t, err.Error(), cfg.IdentityFlagDisabledValue)
				assert.NotContains(t, err.Error(), cfg.IdentityFlagSelectValue)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveIdentity_NoDefaultMessageIsASingleSentence(t *testing.T) {
	mgr := types.NewMockAuthManager(gomock.NewController(t))
	mgr.EXPECT().GetIdentities().Return(identitiesWithDefaults())

	_, err := ResolveIdentity(mgr, "")
	require.ErrorIs(t, err, errUtils.ErrNoDefaultIdentity)
	assert.Equal(t, errUtils.ErrNoDefaultIdentity.Error(), err.Error(), "the message must not repeat itself")
}

func TestResolveIdentity_MultipleDefaultsAreListedSorted(t *testing.T) {
	mgr := types.NewMockAuthManager(gomock.NewController(t))
	mgr.EXPECT().GetIdentities().Return(identitiesWithDefaults("b-default", "a-default"))

	_, err := ResolveIdentity(mgr, "")
	require.ErrorIs(t, err, errUtils.ErrMultipleDefaultIdentities)
	assert.Contains(t, err.Error(), "a-default, b-default")
}

func TestProduce_PassesMinValidityToStandaloneIdentities(t *testing.T) {
	tests := []struct {
		name string
		opts []Option
		want time.Duration
	}{
		{name: "default", want: DefaultMinValidity},
		{name: "explicit", opts: []Option{WithMinValidity(30 * time.Minute)}, want: 30 * time.Minute},
		{name: "zero is passed through, not replaced by the default", opts: []Option{WithMinValidity(0)}, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := types.NewMockAuthManager(gomock.NewController(t))
			mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(nil, errUtils.ErrNoCredentialsFound)
			mgr.EXPECT().Authenticate(gomock.Any(), testIdentity).DoAndReturn(
				func(ctx context.Context, _ string) (*types.WhoamiInfo, error) {
					got, ok := types.MinCredentialValidity(ctx)
					assert.True(t, ok, "the requested validity must reach the identity")
					assert.Equal(t, tt.want, got)
					return whoami(awsCreds("")), nil
				},
			)

			_, err := Produce(context.Background(), mgr, testIdentity, tt.opts...)
			require.NoError(t, err)
		})
	}
}

func TestProduce_SourceCannotSatisfyMinValidity_ReturnsWhatItGot(t *testing.T) {
	// The helper only issues credentials that are valid for 20 minutes. Asking for 30 forces one
	// refresh, and then the short-lived credentials are returned instead of looping or failing.
	mgr := types.NewMockAuthManager(gomock.NewController(t))
	short := fixedNow.Add(20 * time.Minute).Format(time.RFC3339)
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(whoami(awsCreds(short)), nil)
	mgr.EXPECT().Authenticate(gomock.Any(), testIdentity).Return(whoami(awsCreds(short)), nil).Times(1)

	out, err := Produce(context.Background(), mgr, testIdentity, WithClock(fixedClock), WithMinValidity(30*time.Minute))
	require.NoError(t, err)
	assert.Contains(t, string(out), `"Expiration":"2026-10-02T12:20:00Z"`)
}

func TestProduce_AuthenticateHintsSurviveToFormattedOutput(t *testing.T) {
	// The manager's "identity not found" error carries an explanation and a hint (and, when the
	// identity lives in another profile, a profile hint). Wrapping must not drop them.
	const hint = "Run `atmos auth list` to see available identities"
	notFound := errUtils.Build(errUtils.ErrIdentityNotFound).
		WithExplanationf("Identity `%s` is not defined in the currently loaded auth config.", testIdentity).
		WithHint(hint).
		Err()

	mgr := types.NewMockAuthManager(gomock.NewController(t))
	mgr.EXPECT().GetCachedCredentials(gomock.Any(), testIdentity).Return(nil, errUtils.ErrIdentityNotFound)
	mgr.EXPECT().Authenticate(gomock.Any(), testIdentity).Return(nil, notFound)

	_, err := Produce(context.Background(), mgr, testIdentity)
	require.ErrorIs(t, err, errUtils.ErrIdentityAuthFailed)
	require.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
	assert.Contains(t, err.Error(), testIdentity)
	assert.Contains(t, errUtils.AllHints(err), hint)

	formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
	assert.Contains(t, formatted, "atmos auth list")
	assert.Contains(t, formatted, "not defined in the currently loaded auth config")
}

func TestWriteFile(t *testing.T) {
	doc := []byte(`{"Version":1,"AccessKeyId":"AKIAEXAMPLEKEY","SecretAccessKey":"secret"}`)

	t.Run("creates the file owner-readable only", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "creds.json")
		require.NoError(t, WriteFile(path, doc))

		got, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, Render(doc), string(got))
		assertOwnerOnly(t, path)
	})

	t.Run("replaces an existing world-readable file without ever exposing the secrets in it", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "creds.json")
		require.NoError(t, os.WriteFile(path, []byte("stale content that is longer than the new document, stale stale stale"), 0o644))
		require.NoError(t, os.Chmod(path, 0o644)) // Defeat the umask.

		require.NoError(t, WriteFile(path, doc))

		got, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, Render(doc), string(got), "no stale tail may remain")
		assertOwnerOnly(t, path)

		// Only the target remains: the temp file was renamed over it.
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, "creds.json", entries[0].Name())
	})

	t.Run("a failed write reports ErrWriteFile and leaves no temp file or secrets behind", func(t *testing.T) {
		dir := t.TempDir()
		// A directory at the target makes the final rename fail on every platform.
		target := filepath.Join(dir, "creds.json")
		require.NoError(t, os.Mkdir(target, 0o755))

		err := WriteFile(target, doc)
		require.ErrorIs(t, err, errUtils.ErrWriteFile)

		entries, readErr := os.ReadDir(dir)
		require.NoError(t, readErr)
		require.Len(t, entries, 1, "the temp file must be cleaned up")
		assert.Equal(t, "creds.json", entries[0].Name())
	})

	t.Run("a missing directory reports ErrWriteFile", func(t *testing.T) {
		err := WriteFile(filepath.Join(t.TempDir(), "missing", "creds.json"), doc)
		require.ErrorIs(t, err, errUtils.ErrWriteFile)
	})
}

// assertOwnerOnly asserts the file is not accessible to group or others. Windows does not model
// POSIX permission bits, so the check only applies elsewhere.
func assertOwnerOnly(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestParseMinValidity(t *testing.T) {
	tests := []struct {
		value   string
		want    time.Duration
		wantErr bool
	}{
		{value: "15m", want: 15 * time.Minute},
		{value: "1h30m", want: 90 * time.Minute},
		{value: "0", want: 0},
		{value: "soon", wantErr: true},
		{value: "-5m", wantErr: true},
		{value: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := ParseMinValidity(tt.value)
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatMinValidity(t *testing.T) {
	tests := map[time.Duration]string{
		DefaultMinValidity:           "15m",
		time.Hour:                    "1h",
		90 * time.Minute:             "1h30m",
		30 * time.Second:             "30s",
		time.Hour + 5*time.Second:    "1h0m5s",
		0:                            "0s",
		15*time.Minute + time.Second: "15m1s",
	}
	for d, want := range tests {
		assert.Equal(t, want, FormatMinValidity(d))
	}
	// The displayed default always parses back to the producer's default.
	parsed, err := time.ParseDuration(FormatMinValidity(DefaultMinValidity))
	require.NoError(t, err)
	assert.Equal(t, DefaultMinValidity, parsed)
}
