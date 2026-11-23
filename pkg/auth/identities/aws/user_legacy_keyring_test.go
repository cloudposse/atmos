package aws

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosCredentials "github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/credentials/credtest"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields these tests rely on.
var _ = schema.Identity{Kind: "aws/user", Spec: map[string]any{}, Credentials: map[string]any{}}

const (
	// Realm "acme" + identity "prod_admin" and realm "acme_prod" + identity "admin" both used
	// the previous-layout keyring key "atmos_acme_prod_admin".
	legacyTestRealm    = "acme"
	legacyTestIdentity = "prod_admin"
	legacyTestKey      = "atmos_acme_prod_admin"
	legacyTestAccessID = "AKIALEGACYEXAMPLE"
)

// legacyKeyringFixture is an aws/user identity over an isolated file keyring that also exposes
// the raw keyring, so tests can seed and inspect keys exactly as an older Atmos wrote them.
type legacyKeyringFixture struct {
	identity *userIdentity
	store    types.CredentialStore
	raw      keyring.Keyring
	prompter *MocklegacyKeyringPrompter
	sts      *legacySTSRecorder
}

// legacySTSRecorder is an STS endpoint that answers GetSessionToken and records the access key that
// signed each request.
type legacySTSRecorder struct {
	mu         sync.Mutex
	accessKeys []string
}

func (r *legacySTSRecorder) signedWith() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.accessKeys...)
}

func newLegacySTSRecorder(t *testing.T) (*legacySTSRecorder, string) {
	t.Helper()
	rec := &legacySTSRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// SigV4: "Credential=<access key id>/<date>/<region>/sts/aws4_request".
		auth := r.Header.Get("Authorization")
		if _, after, ok := strings.Cut(auth, "Credential="); ok {
			accessKey, _, _ := strings.Cut(after, "/")
			rec.mu.Lock()
			rec.accessKeys = append(rec.accessKeys, accessKey)
			rec.mu.Unlock()
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprint(w, `<GetSessionTokenResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetSessionTokenResult>
    <Credentials>
      <AccessKeyId>ASIASESSIONEXAMPLE</AccessKeyId>
      <SecretAccessKey>session-secret</SecretAccessKey>
      <SessionToken>session-token</SessionToken>
      <Expiration>2099-01-01T00:00:00Z</Expiration>
    </Credentials>
  </GetSessionTokenResult>
  <ResponseMetadata><RequestId>legacy-keyring-test</RequestId></ResponseMetadata>
</GetSessionTokenResponse>`)
	}))
	t.Cleanup(srv.Close)
	return rec, srv.URL
}

func newLegacyKeyringFixture(t *testing.T, webflow bool) *legacyKeyringFixture {
	t.Helper()
	credtest.IsolateKeyring(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	resetLegacyKeyringDeclines(t)

	rec, url := newLegacySTSRecorder(t)
	identity, err := NewUserIdentity(legacyTestIdentity, &schema.Identity{
		Kind:        "aws/user",
		Spec:        map[string]any{"endpoint_url": url},
		Credentials: map[string]any{"region": "us-east-1", "webflow_enabled": webflow},
	})
	require.NoError(t, err)
	user := identity.(*userIdentity)
	user.SetRealm(legacyTestRealm)

	store := atmosCredentials.NewCredentialStoreWithConfig(nil)
	require.Equal(t, types.CredentialStoreTypeFile, store.Type(), "tests must never reach the OS keyring")
	user.SetCredentialStore(store)
	raw, err := atmosCredentials.OpenKeyring(nil)
	require.NoError(t, err)

	prompter := NewMocklegacyKeyringPrompter(gomock.NewController(t))
	user.legacyPrompter = prompter
	return &legacyKeyringFixture{identity: user, store: store, raw: raw, prompter: prompter, sts: rec}
}

// resetLegacyKeyringDeclines clears the process-wide "declined" memory before and after a test.
func resetLegacyKeyringDeclines(t *testing.T) {
	t.Helper()
	declinedLegacyKeyringAdoptions.Clear()
	t.Cleanup(declinedLegacyKeyringAdoptions.Clear)
}

// seedLegacy stores creds under the previous-layout key, as an older Atmos version did.
func (f *legacyKeyringFixture) seedLegacy(t *testing.T, creds types.ICredentials) string {
	t.Helper()
	current := currentTestKey()
	require.NoError(t, f.store.Store(legacyTestIdentity, creds, legacyTestRealm))
	data, err := f.raw.Get(current)
	require.NoError(t, err)
	require.NoError(t, f.raw.Delete(current))
	require.NoError(t, f.raw.Set(legacyTestKey, data))
	return data
}

func currentTestKey() string {
	return atmosCredentials.KeyringRealmPrefix + atmosCredentials.KeyringSeparator + legacyTestRealm +
		atmosCredentials.KeyringRealmAliasSeparator + legacyTestIdentity
}

func legacyLongLivedKeys() *types.AWSCredentials {
	return &types.AWSCredentials{AccessKeyID: legacyTestAccessID, SecretAccessKey: "legacy-secret"}
}

func hintsOf(err error) string {
	return strings.Join(cockroachErrors.GetAllHints(err), "\n")
}

func assertKeyringUnchanged(t *testing.T, f *legacyKeyringFixture, legacyData string) {
	t.Helper()
	got, err := f.raw.Get(legacyTestKey)
	require.NoError(t, err, "the legacy entry must be kept")
	assert.Equal(t, legacyData, got, "the legacy entry must not be modified")
	_, err = f.raw.Get(currentTestKey())
	require.ErrorIs(t, err, keyring.ErrNotFound, "nothing may be written under the current key")
}

func TestUserIdentity_LegacyKeyring_YesAdoptsEntryAndAuthenticates(t *testing.T) {
	f := newLegacyKeyringFixture(t, false)
	legacyData := f.seedLegacy(t, legacyLongLivedKeys())
	f.prompter.EXPECT().Available().Return(true)
	f.prompter.EXPECT().ConfirmAdoption(gomock.Any()).DoAndReturn(func(req *legacyKeyringAdoption) (bool, error) {
		assert.Equal(t, legacyTestIdentity, req.Identity)
		assert.Equal(t, legacyTestRealm, req.Realm)
		assert.Equal(t, legacyTestKey, req.Key)
		assert.Equal(t, []atmosCredentials.LegacyRealmOwner{{Realm: "acme_prod", Alias: "admin"}}, req.OtherOwners)
		return true, nil
	})

	creds, err := f.identity.Authenticate(context.Background(), nil)
	require.NoError(t, err)

	session, ok := creds.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, "ASIASESSIONEXAMPLE", session.AccessKeyID)
	assert.Equal(t, []string{legacyTestAccessID}, f.sts.signedWith(), "STS must be called with the adopted long-lived keys")

	moved, err := f.raw.Get(currentTestKey())
	require.NoError(t, err, "the adopted entry must be stored under the current key")
	assert.Equal(t, legacyData, moved, "the entry must be copied unchanged")
	_, err = f.raw.Get(legacyTestKey)
	require.ErrorIs(t, err, keyring.ErrNotFound, "a confirmed legacy entry is removed after the move")
	stored, err := f.store.Retrieve(legacyTestIdentity, legacyTestRealm)
	require.NoError(t, err)
	assert.Equal(t, legacyLongLivedKeys(), stored)
}

func TestUserIdentity_LegacyKeyring_NoReturnsHintedErrorAndKeepsKeyring(t *testing.T) {
	f := newLegacyKeyringFixture(t, false)
	legacyData := f.seedLegacy(t, legacyLongLivedKeys())
	// Exactly one prompt: the answer is remembered for the rest of the invocation.
	f.prompter.EXPECT().Available().Return(true).Times(1)
	f.prompter.EXPECT().ConfirmAdoption(gomock.Any()).Return(false, nil).Times(1)

	for attempt := 1; attempt <= 2; attempt++ {
		creds, err := f.identity.Authenticate(context.Background(), nil)
		require.Error(t, err, "attempt %d", attempt)
		assert.Nil(t, creds)
		require.ErrorIs(t, err, errUtils.ErrAwsUserNotConfigured)
		require.ErrorIs(t, err, errUtils.ErrAwsUserLegacyKeyringEntryNotAdopted)
		assert.Contains(t, hintsOf(err), "atmos auth user configure --identity=prod_admin")
		assert.NotContains(t, hintsOf(err), "interactive terminal", "the user already answered; do not suggest re-running interactively")
	}
	assertKeyringUnchanged(t, f, legacyData)
	assert.Empty(t, f.sts.signedWith(), "a declined entry must never be used")
}

func TestUserIdentity_LegacyKeyring_NonInteractiveNeverPrompts(t *testing.T) {
	tests := []struct {
		name      string
		ctx       context.Context
		available bool
	}{
		{name: "no terminal", ctx: context.Background(), available: false},
		{name: "prompts disabled by the caller", ctx: types.WithAllowPrompts(context.Background(), false), available: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newLegacyKeyringFixture(t, false)
			legacyData := f.seedLegacy(t, legacyLongLivedKeys())
			f.prompter.EXPECT().Available().Return(tt.available).AnyTimes()
			f.prompter.EXPECT().ConfirmAdoption(gomock.Any()).Times(0)

			_, err := f.identity.Authenticate(tt.ctx, nil)

			require.ErrorIs(t, err, errUtils.ErrAwsUserNotConfigured)
			require.ErrorIs(t, err, errUtils.ErrAwsUserLegacyKeyringEntryNotAdopted)
			hints := hintsOf(err)
			assert.Contains(t, hints, "atmos auth user configure --identity=prod_admin")
			assert.Contains(t, hints, legacyTestKey, "the hint must name the legacy key")
			assert.Contains(t, hints, "interactive terminal")
			assert.Contains(t, strings.Join(errUtils.AllDetails(err), "\n"), "identity `admin` in realm `acme_prod`")
			assertKeyringUnchanged(t, f, legacyData)
		})
	}
}

func TestUserIdentity_LegacyKeyring_NoPromptWhenCurrentKeyExists(t *testing.T) {
	f := newLegacyKeyringFixture(t, false)
	f.seedLegacy(t, legacyLongLivedKeys())
	current := &types.AWSCredentials{AccessKeyID: "AKIACURRENTEXAMPLE", SecretAccessKey: "current-secret"}
	require.NoError(t, f.store.Store(legacyTestIdentity, current, legacyTestRealm))
	// No prompter expectations: any call fails the test.

	_, err := f.identity.Authenticate(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"AKIACURRENTEXAMPLE"}, f.sts.signedWith())
}

func TestUserIdentity_LegacyKeyring_NoPromptWithoutLegacyKey(t *testing.T) {
	f := newLegacyKeyringFixture(t, false)

	_, err := f.identity.Authenticate(types.WithAllowPrompts(context.Background(), false), nil)

	require.ErrorIs(t, err, errUtils.ErrAwsUserNotConfigured)
	assert.NotErrorIs(t, err, errUtils.ErrAwsUserLegacyKeyringEntryNotAdopted)
}

// Session credentials (what other identity kinds cache) are never offered: logging in again
// recreates them, so only long-lived aws/user keys justify the question.
func TestUserIdentity_LegacyKeyring_SessionEntryIsNotOffered(t *testing.T) {
	f := newLegacyKeyringFixture(t, false)
	f.seedLegacy(t, &types.AWSCredentials{AccessKeyID: "ASIAOTHER", SecretAccessKey: "s", SessionToken: "token"})

	_, err := f.identity.Authenticate(types.WithAllowPrompts(context.Background(), false), nil)

	require.ErrorIs(t, err, errUtils.ErrAwsUserNotConfigured)
	assert.NotErrorIs(t, err, errUtils.ErrAwsUserLegacyKeyringEntryNotAdopted)
}

// Without a realm there is no previous realm layout, so nothing is offered.
func TestUserIdentity_LegacyKeyring_NoRealmNeverPrompts(t *testing.T) {
	f := newLegacyKeyringFixture(t, false)
	f.seedLegacy(t, legacyLongLivedKeys())
	f.identity.SetRealm("")

	_, err := f.identity.Authenticate(context.Background(), nil)

	require.ErrorIs(t, err, errUtils.ErrAwsUserNotConfigured)
	assert.NotErrorIs(t, err, errUtils.ErrAwsUserLegacyKeyringEntryNotAdopted)
}

// When browser sign-in is enabled (the default), declining or skipping the entry keeps the
// existing webflow fallback; when that also fails, the legacy-key hints are kept.
func TestUserIdentity_LegacyKeyring_WebflowFailureKeepsLegacyHints(t *testing.T) {
	f := newLegacyKeyringFixture(t, true)
	f.seedLegacy(t, legacyLongLivedKeys())
	f.prompter.EXPECT().Available().Return(false).AnyTimes()

	// With prompts disabled, the webflow fails fast instead of waiting for a browser.
	_, err := f.identity.Authenticate(types.WithAllowPrompts(context.Background(), false), nil)

	require.ErrorIs(t, err, errUtils.ErrWebflowAuthFailed)
	require.ErrorIs(t, err, errUtils.ErrAwsUserLegacyKeyringEntryNotAdopted)
	assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), legacyTestKey)
}

func TestUserIdentity_LegacyKeyring_PromptFailures(t *testing.T) {
	t.Run("prompt unavailable is reported like a non-interactive session", func(t *testing.T) {
		f := newLegacyKeyringFixture(t, false)
		legacyData := f.seedLegacy(t, legacyLongLivedKeys())
		f.prompter.EXPECT().Available().Return(true)
		f.prompter.EXPECT().ConfirmAdoption(gomock.Any()).Return(false, errUtils.ErrAuthPromptUnavailable)

		_, err := f.identity.Authenticate(context.Background(), nil)

		require.ErrorIs(t, err, errUtils.ErrAwsUserLegacyKeyringEntryNotAdopted)
		assert.Contains(t, hintsOf(err), "interactive terminal")
		assertKeyringUnchanged(t, f, legacyData)
	})

	t.Run("cancelled prompt aborts without changes", func(t *testing.T) {
		f := newLegacyKeyringFixture(t, false)
		legacyData := f.seedLegacy(t, legacyLongLivedKeys())
		f.prompter.EXPECT().Available().Return(true)
		f.prompter.EXPECT().ConfirmAdoption(gomock.Any()).Return(false, errUtils.ErrUserAborted)

		_, err := f.identity.Authenticate(context.Background(), nil)

		require.ErrorIs(t, err, errUtils.ErrUserAborted)
		require.ErrorIs(t, err, errUtils.ErrAwsUserNotConfigured)
		assertKeyringUnchanged(t, f, legacyData)
	})

	t.Run("entry removed before the move", func(t *testing.T) {
		f := newLegacyKeyringFixture(t, false)
		f.seedLegacy(t, legacyLongLivedKeys())
		f.prompter.EXPECT().Available().Return(true)
		f.prompter.EXPECT().ConfirmAdoption(gomock.Any()).DoAndReturn(func(*legacyKeyringAdoption) (bool, error) {
			// Another process removed the entry while the question was on screen.
			require.NoError(t, f.raw.Delete(legacyTestKey))
			return true, nil
		})

		_, err := f.identity.Authenticate(context.Background(), nil)

		require.ErrorIs(t, err, errUtils.ErrAwsUserKeyringReadFailed)
		require.ErrorIs(t, err, atmosCredentials.ErrCredentialsNotFound)
		assert.Empty(t, f.sts.signedWith())
	})
}

// An aborted adoption prompt must not start the browser webflow, and a negative path (a plain
// not-configured error) keeps the webflow fallback.
func TestUserIdentity_ShouldFallBackToWebflow_AbortedPrompt(t *testing.T) {
	f := newLegacyKeyringFixture(t, true)

	aborted := fmt.Errorf("%w: confirming the older keyring entry: %w", errUtils.ErrAwsUserNotConfigured, errUtils.ErrUserAborted)
	assert.False(t, f.identity.shouldFallBackToWebflow(aborted))
	assert.True(t, f.identity.shouldFallBackToWebflow(errUtils.ErrAwsUserNotConfigured))
}

// Credentials that appear under the current key while the confirmation is open go through the
// session-token validation instead of being used as long-lived keys.
func TestUserIdentity_LegacyKeyring_AdoptedSessionCredentialsAreValidated(t *testing.T) {
	f := newLegacyKeyringFixture(t, false)
	f.seedLegacy(t, legacyLongLivedKeys())
	f.prompter.EXPECT().Available().Return(true)
	f.prompter.EXPECT().ConfirmAdoption(gomock.Any()).DoAndReturn(func(*legacyKeyringAdoption) (bool, error) {
		// Another process stores session credentials under the current key meanwhile.
		require.NoError(t, f.store.Store(legacyTestIdentity,
			&types.AWSCredentials{AccessKeyID: "ASIAOTHER", SecretAccessKey: "s", SessionToken: "token"}, legacyTestRealm))
		return true, nil
	})

	_, err := f.identity.Authenticate(context.Background(), nil)

	require.ErrorIs(t, err, errUtils.ErrAwsUserNotConfigured)
	assert.Empty(t, f.sts.signedWith(), "session credentials must not be used as long-lived keys")
}

func TestLegacyKeyringPromptText(t *testing.T) {
	req := &legacyKeyringAdoption{
		Identity:    legacyTestIdentity,
		Realm:       legacyTestRealm,
		Key:         legacyTestKey,
		OtherOwners: []atmosCredentials.LegacyRealmOwner{{Realm: "acme_prod", Alias: "admin"}},
	}

	assert.Equal(t, `Use the older keyring entry for identity "prod_admin"?`, legacyKeyringPromptTitle(req))
	description := legacyKeyringPromptDescription(req)
	for _, want := range []string{`identity "prod_admin" in realm "acme"`, `keyring key "atmos_acme_prod_admin"`, "older Atmos version", `identity "admin" in realm "acme_prod"`, "Choose Yes only"} {
		assert.Contains(t, description, want)
	}
	assert.NotContains(t, description, "—", "no em-dashes in user-facing text")

	assert.Equal(t, `identity "a" in realm "r1" or identity "b" in realm "r2"`,
		describeLegacyOwners([]atmosCredentials.LegacyRealmOwner{{Realm: "r1", Alias: "a"}, {Realm: "r2", Alias: "b"}}, false))
	assert.Equal(t, "identity `a` in realm `r1`", describeLegacyOwners([]atmosCredentials.LegacyRealmOwner{{Realm: "r1", Alias: "a"}}, true))
	assert.Contains(t, describeLegacyOwners(nil, true), "other realm")
}

// The production prompter must fail fast instead of drawing an invisible form.
func TestHuhLegacyKeyringPrompter_FailsFastWhenNotInteractive(t *testing.T) {
	disableInteractive(t)
	prompter := huhLegacyKeyringPrompter{}

	assert.False(t, prompter.Available())
	adopt, err := prompter.ConfirmAdoption(&legacyKeyringAdoption{Identity: "x"})
	require.ErrorIs(t, err, errUtils.ErrAuthPromptUnavailable)
	assert.False(t, adopt, "the default answer is No")
}

func TestUserIdentity_LegacyPrompterOrDefault(t *testing.T) {
	user := newPromptTestUser(t, "dev")
	assert.IsType(t, huhLegacyKeyringPrompter{}, user.legacyPrompterOrDefault())
}
