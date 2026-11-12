package github_test

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/authority/authoritytest"
	"github.com/cloudposse/atmos/plugins/ssh/authority/github"
)

func newAuthority(t *testing.T, srv *authoritytest.GitHubServer) *github.Authority {
	t.Helper()
	a, err := github.New(authority.Options{BaseURL: srv.URL, HTTPClient: srv.Client(), UserAgent: "atmos-test/1"})
	require.NoError(t, err)
	return a
}

func creds(token string) map[string]string {
	return map[string]string{authority.CredentialToken: token}
}

func req(key ssh.PublicKey, uses ...authority.Use) authority.Request {
	return authority.Request{KeyName: "ops", Key: key, Uses: uses, Title: "ops@host (Atmos)"}
}

func TestConformance(t *testing.T) {
	authoritytest.Run(t, func(t *testing.T) *authoritytest.Environment {
		srv := authoritytest.NewGitHubServer(t)
		return srv.Environment(newAuthority(t, srv))
	})
}

func TestConformancePaginated(t *testing.T) {
	authoritytest.Run(t, func(t *testing.T) *authoritytest.Environment {
		srv := authoritytest.NewGitHubServer(t)
		srv.PageSize = 1
		return srv.Environment(newAuthority(t, srv))
	})
}

func TestConformanceForbiddenForMissingScope(t *testing.T) {
	authoritytest.Run(t, func(t *testing.T) *authoritytest.Environment {
		srv := authoritytest.NewGitHubServer(t)
		srv.MissingScopeStatus = http.StatusForbidden
		return srv.Environment(newAuthority(t, srv))
	})
}

func TestHeadersOnEveryCall(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a := newAuthority(t, srv)
	env := srv.Environment(a)
	key := authoritytest.NewKey(t)

	receipt, err := a.Ensure(context.Background(), env.Credentials, req(key, authority.UseSigning, authority.UseAuthentication))
	require.NoError(t, err)
	require.NoError(t, a.Revoke(context.Background(), env.Credentials, receipt))

	logged := srv.Requests()
	require.Len(t, logged, 7)
	for _, r := range logged {
		assert.True(t, r.HasBearer, "%s %s", r.Method, r.Path)
		assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
		assert.Equal(t, "2022-11-28", r.Header.Get("X-GitHub-Api-Version"))
		assert.Equal(t, "atmos-test/1", r.Header.Get("User-Agent"))
	}
}

func TestDefaultUserAgentIsSent(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a, err := github.New(authority.Options{BaseURL: srv.URL})
	require.NoError(t, err)
	env := srv.Environment(a)
	_, err = a.Ensure(context.Background(), env.Credentials, req(authoritytest.NewKey(t), authority.UseSigning))
	require.NoError(t, err)
	assert.Equal(t, github.DefaultUserAgent, srv.Requests()[0].Header.Get("User-Agent"))
}

func TestEnsureRecordsLoginAndIDs(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a := newAuthority(t, srv)
	env := srv.Environment(a)
	key := authoritytest.NewKey(t)

	receipt, err := a.Ensure(context.Background(), env.Credentials, req(key, authority.UseSigning, authority.UseAuthentication))
	require.NoError(t, err)

	assert.Equal(t, authoritytest.Login, receipt.Subject)
	signing := srv.Keys(authoritytest.Login, authority.UseSigning)
	auth := srv.Keys(authoritytest.Login, authority.UseAuthentication)
	require.Len(t, signing, 1)
	require.Len(t, auth, 1)
	assert.Equal(t, strconv.FormatInt(signing[0].ID, 10), receipt.References[authority.UseSigning])
	assert.Equal(t, strconv.FormatInt(auth[0].ID, 10), receipt.References[authority.UseAuthentication])
	assert.Equal(t, "ops@host (Atmos)", signing[0].Title)
}

func TestEmptyTitleGetsDefault(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a := newAuthority(t, srv)
	env := srv.Environment(a)
	_, err := a.Ensure(context.Background(), env.Credentials, authority.Request{KeyName: "k", Key: authoritytest.NewKey(t), Uses: []authority.Use{authority.UseSigning}})
	require.NoError(t, err)
	assert.NotEmpty(t, srv.Keys(authoritytest.Login, authority.UseSigning)[0].Title)
}

func TestFindsKeyOnLaterPageAndIgnoresComment(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	srv.PageSize = 2
	a := newAuthority(t, srv)
	env := srv.Environment(a)

	for range 5 {
		srv.AddKey(authoritytest.Login, authority.UseAuthentication, authoritytest.NewKey(t), "other")
	}
	key := authoritytest.NewKey(t)
	want := srv.AddKey(authoritytest.Login, authority.UseAuthentication, key, "mine")
	srv.AddKey(authoritytest.Login, authority.UseAuthentication, authoritytest.NewKey(t), "after")
	srv.ResetRequests()

	receipt, err := a.Ensure(context.Background(), env.Credentials, req(key, authority.UseAuthentication))
	require.NoError(t, err)
	assert.Equal(t, strconv.FormatInt(want, 10), receipt.References[authority.UseAuthentication])
	assert.Len(t, srv.Keys(authoritytest.Login, authority.UseAuthentication), 7, "nothing was created")

	var gets int
	for _, r := range srv.Requests() {
		assert.NotEqual(t, http.MethodPost, r.Method)
		if r.Path == "/user/keys" {
			gets++
		}
	}
	assert.Equal(t, 3, gets, "pages 1 to 3 were read")
}

func TestKeyOnOtherAccountIsInUse(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a := newAuthority(t, srv)
	env := srv.Environment(a)
	key := authoritytest.NewKey(t)
	srv.AddKey("someone-else", authority.UseSigning, key, "theirs")

	receipt, err := a.Ensure(context.Background(), env.Credentials, req(key, authority.UseSigning))
	require.Error(t, err)
	assert.ErrorIs(t, err, authority.ErrKeyInUse)
	var ae *authority.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, http.StatusUnprocessableEntity, ae.Status)
	assert.Empty(t, receipt.References)
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		name    string
		failure authoritytest.Failure
		want    error
		check   func(*testing.T, *authority.Error)
	}{
		{
			name:    "bad token on login",
			failure: authoritytest.Failure{Method: "GET", PathPrefix: "/user", Status: 401, Body: `{"message":"Bad credentials"}`},
			want:    authority.ErrUnauthorized,
			check:   func(t *testing.T, e *authority.Error) { assert.Equal(t, 401, e.Status) },
		},
		{
			name:    "forbidden with remaining quota is a missing permission",
			failure: authoritytest.Failure{Method: "POST", PathPrefix: "/user/ssh_signing_keys", Status: 403, Body: `{"message":"Resource not accessible by personal access token"}`},
			want:    authority.ErrMissingPermission,
			check: func(t *testing.T, e *authority.Error) {
				assert.Equal(t, "write:ssh_signing_key", e.Permission)
				assert.Equal(t, authority.UseSigning, e.Use)
			},
		},
		{
			name: "403 with exhausted quota is rate limiting",
			failure: authoritytest.Failure{
				Method: "GET", PathPrefix: "/user/ssh_signing_keys", Status: 403,
				Header: http.Header{"X-Ratelimit-Remaining": {"0"}, "Retry-After": {"90"}},
				Body:   `{"message":"API rate limit exceeded"}`,
			},
			want:  authority.ErrRateLimited,
			check: func(t *testing.T, e *authority.Error) { assert.Equal(t, 90*time.Second, e.RetryAfter) },
		},
		{
			name:    "secondary rate limit by message",
			failure: authoritytest.Failure{Method: "GET", PathPrefix: "/user/ssh_signing_keys", Status: 403, Body: `{"message":"You have exceeded a secondary rate limit"}`},
			want:    authority.ErrRateLimited,
		},
		{
			name:    "429",
			failure: authoritytest.Failure{Method: "GET", PathPrefix: "/user/ssh_signing_keys", Status: 429, Body: `{}`},
			want:    authority.ErrRateLimited,
		},
		{
			name:    "server error",
			failure: authoritytest.Failure{Method: "GET", PathPrefix: "/user/ssh_signing_keys", Status: 502, Body: `<html>bad gateway</html>`},
			want:    authority.ErrUnexpectedStatus,
			check:   func(t *testing.T, e *authority.Error) { assert.Contains(t, e.Detail, "bad gateway") },
		},
		{
			name:    "redirect on a write is not followed",
			failure: authoritytest.Failure{Method: "POST", PathPrefix: "/user/ssh_signing_keys", Status: 307, Header: http.Header{"Location": {"https://evil.example/steal"}}},
			want:    authority.ErrUnexpectedStatus,
			check:   func(t *testing.T, e *authority.Error) { assert.Equal(t, 307, e.Status) },
		},
		{
			name:    "unparseable login",
			failure: authoritytest.Failure{Method: "GET", PathPrefix: "/user", Status: 200, Body: `[]`},
			want:    authority.ErrUnexpectedStatus,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := authoritytest.NewGitHubServer(t)
			a := newAuthority(t, srv)
			env := srv.Environment(a)
			srv.Fail(tt.failure)

			_, err := a.Ensure(context.Background(), env.Credentials, req(authoritytest.NewKey(t), authority.UseSigning))
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
			var ae *authority.Error
			require.ErrorAs(t, err, &ae)
			assert.Equal(t, "github", ae.Authority)
			assert.NotContains(t, err.Error(), authoritytest.FullToken)
			if tt.check != nil {
				tt.check(t, ae)
			}
		})
	}
}

func TestMissingScopeNamesWriteWhenOnlyReadIsGranted(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a := newAuthority(t, srv)
	srv.AddToken("ghp_readonly", "octocat", "read:ssh_signing_key")

	_, err := a.Ensure(context.Background(), creds("ghp_readonly"), req(authoritytest.NewKey(t), authority.UseSigning))
	require.ErrorIs(t, err, authority.ErrMissingPermission)
	var ae *authority.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, "write:ssh_signing_key", ae.Permission)
	assert.Contains(t, ae.Detail, "read:ssh_signing_key", "the token's own scopes are shown")
	assert.NotContains(t, err.Error(), "ghp_readonly")
}

func TestMissingTokenIsUnauthorizedWithoutNetwork(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a := newAuthority(t, srv)
	_, err := a.Ensure(context.Background(), map[string]string{}, req(authoritytest.NewKey(t), authority.UseSigning))
	assert.ErrorIs(t, err, authority.ErrUnauthorized)
	assert.ErrorIs(t, a.Revoke(context.Background(), nil, authority.Receipt{References: map[authority.Use]string{authority.UseSigning: "1"}}), authority.ErrUnauthorized)
	assert.Empty(t, srv.Requests())
}

func TestErrorBodyEchoingTheTokenIsScrubbed(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a := newAuthority(t, srv)
	env := srv.Environment(a)
	srv.Fail(authoritytest.Failure{Method: "GET", PathPrefix: "/user/ssh_signing_keys", Status: 500, Body: `{"message":"token ` + authoritytest.FullToken + ` is weird"}`})

	_, err := a.Ensure(context.Background(), env.Credentials, req(authoritytest.NewKey(t), authority.UseSigning))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), authoritytest.FullToken)
	assert.Contains(t, err.Error(), "[redacted]")
}

func TestUnreachableServer(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a := newAuthority(t, srv)
	env := srv.Environment(a)
	srv.Close()

	_, err := a.Ensure(context.Background(), env.Credentials, req(authoritytest.NewKey(t), authority.UseSigning))
	require.ErrorIs(t, err, authority.ErrUnavailable)
	assert.NotContains(t, err.Error(), authoritytest.FullToken)
}

func TestPartialEnrollmentKeepsWhatWasCreated(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a := newAuthority(t, srv)
	env := srv.Environment(a)
	srv.Fail(authoritytest.Failure{Method: "POST", PathPrefix: "/user/keys", Status: 500, Body: `{"message":"boom"}`})

	// authentication sorts first, so make signing the first and only success.
	key := authoritytest.NewKey(t)
	receipt, err := a.Ensure(context.Background(), env.Credentials, req(key, authority.UseAuthentication, authority.UseSigning))
	require.ErrorIs(t, err, authority.ErrUnexpectedStatus)
	assert.Empty(t, receipt.References, "authentication failed first, so nothing was enrolled")

	srv.Fail(authoritytest.Failure{Method: "POST", PathPrefix: "/user/ssh_signing_keys", Status: 500, Body: `{"message":"boom"}`})
	receipt, err = a.Ensure(context.Background(), env.Credentials, req(key, authority.UseAuthentication, authority.UseSigning))
	require.Error(t, err)
	require.Contains(t, receipt.References, authority.UseAuthentication)
	assert.Equal(t, []authority.Use{authority.UseAuthentication}, receipt.Uses)
	assert.Equal(t, authoritytest.Login, receipt.Subject)
}

func TestRevokeTreatsMissingIDAsSuccess(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a := newAuthority(t, srv)
	env := srv.Environment(a)
	err := a.Revoke(context.Background(), env.Credentials, authority.Receipt{References: map[authority.Use]string{
		authority.UseSigning: "424242", authority.UseAuthentication: "424243",
	}})
	assert.NoError(t, err)
}

func TestRevokeWithoutScopeIsNotSuccess(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a := newAuthority(t, srv)
	env := srv.Environment(a)
	key := authoritytest.NewKey(t)
	receipt, err := a.Ensure(context.Background(), env.Credentials, req(key, authority.UseSigning))
	require.NoError(t, err)

	err = a.Revoke(context.Background(), creds(authoritytest.LimitedToken(authority.UseSigning)), receipt)
	require.ErrorIs(t, err, authority.ErrMissingPermission)
	assert.Len(t, srv.Keys(authoritytest.Login, authority.UseSigning), 1, "the key was not removed")
}

func TestRevokeAttemptsEveryReferenceAndJoinsErrors(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a := newAuthority(t, srv)
	env := srv.Environment(a)
	key := authoritytest.NewKey(t)
	receipt, err := a.Ensure(context.Background(), env.Credentials, req(key, authority.UseSigning, authority.UseAuthentication))
	require.NoError(t, err)
	srv.Fail(authoritytest.Failure{Method: "DELETE", PathPrefix: "/user/keys", Status: 500, Body: `{}`})

	err = a.Revoke(context.Background(), env.Credentials, receipt)
	require.ErrorIs(t, err, authority.ErrUnexpectedStatus)
	assert.Empty(t, srv.Keys(authoritytest.Login, authority.UseSigning), "the other use was still withdrawn")
	assert.Len(t, srv.Keys(authoritytest.Login, authority.UseAuthentication), 1)
}

func TestRevokeRejectsBadReferences(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a := newAuthority(t, srv)
	env := srv.Environment(a)
	for name, refs := range map[string]map[authority.Use]string{
		"path traversal": {authority.UseSigning: "../../repos"},
		"zero":           {authority.UseSigning: "0"},
		"unknown use":    {authority.Use("x"): "5"},
	} {
		t.Run(name, func(t *testing.T) {
			err := a.Revoke(context.Background(), env.Credentials, authority.Receipt{References: refs})
			require.Error(t, err)
			assert.True(t, errors.Is(err, authority.ErrInvalidRequest) || errors.Is(err, authority.ErrUnsupportedUse))
		})
	}
	for _, r := range srv.Requests() {
		assert.NotContains(t, r.Path, "..")
	}
}

func TestRequestTimeoutAppliesWithoutClientTimeout(t *testing.T) {
	srv := authoritytest.NewGitHubServer(t)
	a := newAuthority(t, srv)
	env := srv.Environment(a)
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	_, err := a.Ensure(ctx, env.Credentials, req(authoritytest.NewKey(t), authority.UseSigning))
	require.ErrorIs(t, err, authority.ErrUnavailable)
}

func TestNewRejectsBadBaseURL(t *testing.T) {
	for _, base := range []string{"ftp://example.com", "://nope", "api.github.com"} {
		_, err := github.New(authority.Options{BaseURL: base})
		assert.ErrorIs(t, err, authority.ErrInvalidRequest, base)
	}
	a, err := github.New(authority.Options{})
	require.NoError(t, err)
	assert.Equal(t, "github", a.Name())
	assert.Equal(t, []authority.Use{authority.UseAuthentication, authority.UseSigning}, a.Supports())
}

func TestFactoryRegisters(t *testing.T) {
	registry := authority.NewRegistry()
	require.NoError(t, registry.Register(github.Factory))
	a, err := registry.New("github", authority.Options{})
	require.NoError(t, err)
	assert.Equal(t, github.Name, a.Name())
	_, err = registry.New("github", authority.Options{BaseURL: "bad"})
	assert.ErrorIs(t, err, authority.ErrInvalidRequest)
}
