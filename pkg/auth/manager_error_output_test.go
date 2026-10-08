package auth

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	testSSOHint        = "Use 'aws sso login' to authenticate before running Atmos in headless environments"
	testSSOExplanation = "AWS SSO device flow requires an interactive terminal (TTY) for user authorization"
)

// captureStderr runs fn while os.Stderr is redirected to a pipe and returns what was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	restored := false
	restore := func() {
		if !restored {
			os.Stderr = oldStderr
			restored = true
		}
	}
	t.Cleanup(func() {
		restore()
		_ = w.Close()
		_ = r.Close()
	})

	fn()

	restore()
	require.NoError(t, w.Close())
	var out bytes.Buffer
	_, err = io.Copy(&out, r)
	require.NoError(t, err)
	return out.String()
}

// ssoLikeError mirrors the error the AWS SSO provider returns when no TTY is available:
// a sentinel carrying an explanation and hints.
func ssoLikeError() error {
	return errUtils.Build(errUtils.ErrAuthenticationFailed).
		WithExplanation(testSSOExplanation).
		WithHint(testSSOHint).
		Err()
}

// failingIdentity is a chained identity whose Authenticate always fails.
type failingIdentity struct {
	stubPSIdentity
	err error
}

func (f *failingIdentity) Authenticate(_ context.Context, _ types.ICredentials) (types.ICredentials, error) {
	return nil, f.err
}

// failingStandaloneIdentity is a standalone chain root whose credential source fails.
type failingStandaloneIdentity struct {
	stubPSIdentity
	err error
}

func (f *failingStandaloneIdentity) IsStandalone() bool { return true }
func (f *failingStandaloneIdentity) AuthenticateStandalone(_ context.Context) (types.ICredentials, error) {
	return nil, f.err
}

func newFailingManager(provErr, identityErr error) *manager {
	return &manager{
		config: &schema.AuthConfig{
			Providers: map[string]schema.Provider{"sso": {Kind: "aws/iam-identity-center"}},
			Identities: map[string]schema.Identity{
				"dev": {Kind: "aws/permission-set", Via: &schema.IdentityVia{Provider: "sso"}},
			},
		},
		providers:       map[string]types.Provider{"sso": &testProvider{name: "sso", authErr: provErr}},
		identities:      map[string]types.Identity{"dev": &failingIdentity{stubPSIdentity: stubPSIdentity{provider: "sso"}, err: identityErr}},
		credentialStore: &testStore{data: map[string]any{}, expired: map[string]bool{}},
		validator:       dummyValidator{},
	}
}

func newFailingStandaloneManager(err error) *manager {
	return &manager{
		config: &schema.AuthConfig{
			Identities: map[string]schema.Identity{"cp": {Kind: types.IdentityKindAWSCredentialProcess}},
		},
		providers:       map[string]types.Provider{},
		identities:      map[string]types.Identity{"cp": &failingStandaloneIdentity{err: err}},
		credentialStore: &testStore{data: map[string]any{}, expired: map[string]bool{}},
		validator:       dummyValidator{},
	}
}

// assertCleanAuthError asserts the invariants every authentication failure must satisfy: the
// library prints nothing itself, and the returned error names the identity exactly once without
// stuttering the sentinel text.
func assertCleanAuthError(t *testing.T, stderr string, err error, identity string) {
	t.Helper()
	require.Error(t, err)
	assert.Empty(t, stderr, "the auth library must not print errors; the command boundary renders them once")
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)

	msg := err.Error()
	assert.Equal(t, 1, strings.Count(msg, "authentication failed"), "sentinel text must appear once: %q", msg)
	assert.Equal(t, 1, strings.Count(msg, `"`+identity+`"`), "identity must be named once: %q", msg)
	assert.NotContains(t, msg, "failed to authenticate via credential chain")
}

func TestManager_Authenticate_ProviderFailure_PrintsNothingAndKeepsHints(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	m := newFailingManager(ssoLikeError(), nil)

	var err error
	stderr := captureStderr(t, func() {
		_, err = m.Authenticate(context.Background(), "dev")
	})

	assertCleanAuthError(t, stderr, err, "dev")
	assert.Contains(t, errUtils.AllHints(err), testSSOHint)
	assert.Contains(t, errUtils.AllDetails(err), testSSOExplanation)

	rendered := errUtils.Format(err, errUtils.DefaultFormatterConfig())
	assert.Contains(t, rendered, testSSOHint)
	assert.Contains(t, rendered, testSSOExplanation)
	assert.Equal(t, 1, strings.Count(rendered, "Error:"), "exactly one error block")
}

func TestManager_Authenticate_IdentityFailure_PrintsNothing(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	cause := errors.New("assume role denied")
	m := newFailingManager(nil, cause)

	var err error
	stderr := captureStderr(t, func() {
		_, err = m.Authenticate(context.Background(), "dev")
	})

	assertCleanAuthError(t, stderr, err, "dev")
	assert.ErrorIs(t, err, cause)
	assert.Contains(t, err.Error(), "assume role denied")
}

func TestManager_Authenticate_StandaloneRootFailure_PrintsNothing(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	cause := errors.New("exit status 1")
	m := newFailingStandaloneManager(cause)

	var err error
	stderr := captureStderr(t, func() {
		_, err = m.Authenticate(context.Background(), "cp")
	})

	assertCleanAuthError(t, stderr, err, "cp")
	assert.ErrorIs(t, err, cause)
	assert.Contains(t, err.Error(), "exit status 1")
}

func TestManager_Authenticate_UnregisteredProvider_PrintsNothing(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	m := newFailingManager(nil, nil)
	m.providers = map[string]types.Provider{}

	var err error
	stderr := captureStderr(t, func() {
		_, err = m.Authenticate(context.Background(), "dev")
	})

	require.Error(t, err)
	assert.Empty(t, stderr)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
}

func TestManager_Authenticate_ChainBuildFailure_PrintsNothing(t *testing.T) {
	resetProcessCredentialCache()
	t.Cleanup(resetProcessCredentialCache)

	m := newFailingManager(nil, nil)
	// An identity with no via and a non-standalone kind cannot form a chain.
	m.config.Identities["dev"] = schema.Identity{Kind: "aws/permission-set"}

	var err error
	stderr := captureStderr(t, func() {
		_, err = m.Authenticate(context.Background(), "dev")
	})

	require.Error(t, err)
	assert.Empty(t, stderr)
	assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
}

func TestManager_GetProviderKindForIdentity_FailurePrintsNothing(t *testing.T) {
	m := newFailingManager(nil, nil)
	m.config.Identities["dev"] = schema.Identity{Kind: "aws/permission-set"}

	var err error
	stderr := captureStderr(t, func() {
		_, err = m.GetProviderKindForIdentity("dev")
	})

	require.Error(t, err)
	assert.Empty(t, stderr)
	assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
}

func TestNewAuthManager_InvalidParams_PrintNothing(t *testing.T) {
	tests := []struct {
		name  string
		build func() error
	}{
		{"nil config", func() error { _, err := NewAuthManager(nil, &testStore{}, dummyValidator{}, nil, ""); return err }},
		{"nil store", func() error {
			_, err := NewAuthManager(&schema.AuthConfig{}, nil, dummyValidator{}, nil, "")
			return err
		}},
		{"nil validator", func() error { _, err := NewAuthManager(&schema.AuthConfig{}, &testStore{}, nil, nil, ""); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			stderr := captureStderr(t, func() { err = tt.build() })
			assert.ErrorIs(t, err, errUtils.ErrNilParam)
			assert.Empty(t, stderr)
		})
	}
}
