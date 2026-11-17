package aws

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/logincreds"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestValidatedContainerDialerDialsOnlyValidatedAddresses(t *testing.T) {
	for _, test := range []struct {
		name      string
		address   string
		resolved  []string
		lookupErr error
		wantDial  []string
		wantErr   error
	}{
		{name: "host resolving to loopback", address: "credentials.internal:8080", resolved: []string{"127.0.0.1"}, wantDial: []string{"127.0.0.1:8080"}},
		{name: "host resolving to the ECS address", address: "ecs.internal:80", resolved: []string{"169.254.170.2"}, wantDial: []string{"169.254.170.2:80"}},
		{name: "IP literal skips the lookup", address: "127.0.0.1:9000", wantDial: []string{"127.0.0.1:9000"}},
		{name: "host resolving outside the allowlist", address: "evil.example:80", resolved: []string{"203.0.113.9"}, wantErr: errUtils.ErrInvalidAuthConfig},
		{name: "one rebound address among allowed ones", address: "rebind.example:80", resolved: []string{"127.0.0.1", "203.0.113.9"}, wantErr: errUtils.ErrInvalidAuthConfig},
		{name: "lookup failure", address: "missing.example:80", lookupErr: errors.New("no such host"), wantErr: errUtils.ErrInvalidAuthConfig},
		{name: "malformed address", address: "no-port", wantErr: errUtils.ErrInvalidAuthConfig},
	} {
		t.Run(test.name, func(t *testing.T) {
			var dialed []string
			lookup := func(context.Context, string) ([]string, error) { return test.resolved, test.lookupErr }
			dial := func(_ context.Context, _, address string) (net.Conn, error) {
				dialed = append(dialed, address)
				client, server := net.Pipe()
				t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
				return client, nil
			}

			conn, err := validatedContainerDialer(lookup, dial)(t.Context(), "tcp", test.address)

			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				assert.Empty(t, dialed, "nothing is dialed for an address that failed validation")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, conn)
			assert.Equal(t, test.wantDial, dialed, "the dial uses the validated address, not the host name")
		})
	}
}

func TestValidatedContainerDialerTriesNextValidatedAddress(t *testing.T) {
	var dialed []string
	lookup := func(context.Context, string) ([]string, error) { return []string{"127.0.0.1", "::1"}, nil }
	dial := func(_ context.Context, _, address string) (net.Conn, error) {
		dialed = append(dialed, address)
		if len(dialed) == 1 {
			return nil, errors.New("connection refused")
		}
		client, server := net.Pipe()
		t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
		return client, nil
	}

	_, err := validatedContainerDialer(lookup, dial)(t.Context(), "tcp", "credentials.internal:80")

	require.NoError(t, err)
	assert.Equal(t, []string{"127.0.0.1:80", "[::1]:80"}, dialed)
}

// writeLoginToken caches an unexpired login token for the session, so no refresh request is needed.
func writeLoginToken(t *testing.T, session, dir, accessKeyID string) {
	t.Helper()
	path, err := logincreds.StandardCachedTokenFilepath(session, dir)
	require.NoError(t, err)
	token := map[string]any{
		"accessToken": map[string]any{
			"accessKeyId": accessKeyID, "secretAccessKey": "secret", "sessionToken": "session", "accountId": "123456789012",
			"expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		},
		"clientId": "client", "refreshToken": "refresh", "dpopKey": "key",
	}
	data, err := json.Marshal(token)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func TestLoginSessionIsResolvedFromTheInvocationEnvironment(t *testing.T) {
	session := "arn:aws:iam::123456789012:user/example"
	root := &config.SharedConfig{LoginSession: session, Region: "us-east-1"}
	assert.True(t, profileSourceNeedsEnvironment(root), "the SDK would read the login cache from the process environment")

	// The process environment names a different cache that holds other credentials.
	processCache := t.TempDir()
	writeLoginToken(t, session, processCache, "AKIAPROCESSCACHE")
	t.Setenv("AWS_LOGIN_CACHE_DIRECTORY", processCache)

	t.Run("explicit cache directory", func(t *testing.T) {
		dir := t.TempDir()
		writeLoginToken(t, session, dir, "AKIAINVOCATION")
		provider, err := loginSessionCredentials(map[string]string{"AWS_LOGIN_CACHE_DIRECTORY": dir}, root, "")
		require.NoError(t, err)

		retrieved, err := provider.Retrieve(t.Context())

		require.NoError(t, err)
		assert.Equal(t, "AKIAINVOCATION", retrieved.AccessKeyID)
	})

	t.Run("invocation home", func(t *testing.T) {
		home := t.TempDir()
		writeLoginToken(t, session, filepath.Join(home, ".aws", "login", "cache"), "AKIAHOME")
		provider, err := loginSessionCredentials(map[string]string{"HOME": home, "USERPROFILE": home}, root, "")
		require.NoError(t, err)

		retrieved, err := provider.Retrieve(t.Context())

		require.NoError(t, err)
		assert.Equal(t, "AKIAHOME", retrieved.AccessKeyID, "the process's AWS_LOGIN_CACHE_DIRECTORY is not consulted")
	})

	t.Run("no cached token", func(t *testing.T) {
		provider, err := loginSessionCredentials(map[string]string{"AWS_LOGIN_CACHE_DIRECTORY": t.TempDir()}, root, "")
		require.NoError(t, err)

		_, err = provider.Retrieve(t.Context())

		require.Error(t, err)
	})
}

func TestContainerEndpointClientBoundsRequests(t *testing.T) {
	release := make(chan struct{})
	hanging := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case <-release:
		case <-time.After(30 * time.Second):
		}
	}))
	// The handler is released before Close, which waits for outstanding requests.
	t.Cleanup(hanging.Close)
	t.Cleanup(func() { close(release) })

	answering := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(answering.Close)

	t.Run("an endpoint that never answers is abandoned after the timeout", func(t *testing.T) {
		client := containerEndpointClient(200 * time.Millisecond)
		start := time.Now()
		resp, err := client.Get(hanging.URL)
		if resp != nil {
			_ = resp.Body.Close()
		}
		require.Error(t, err)
		assert.Less(t, time.Since(start), 10*time.Second)
	})

	t.Run("an endpoint that answers within the timeout still works", func(t *testing.T) {
		client := containerEndpointClient(5 * time.Second)
		resp, err := client.Get(answering.URL)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestLoginSessionWithoutInvocationHomeDoesNotReadProcessCache(t *testing.T) {
	const session = "arn:aws:iam::123456789012:user/example"
	processHome := t.TempDir()
	processCache := filepath.Join(processHome, ".aws", "login", "cache")
	writeLoginToken(t, session, processCache, "AKIAPROCESSCACHE")
	t.Setenv("HOME", processHome)
	t.Setenv("USERPROFILE", processHome)
	t.Setenv("AWS_LOGIN_CACHE_DIRECTORY", processCache)
	home := homeWithProfiles(t, "", "[default]\nlogin_session = "+session+"\nregion = us-east-1\n")
	env := map[string]string{"AWS_CONFIG_FILE": filepath.Join(home, ".aws", "config")}

	value, err := baseCredentials(t.Context(), env)
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
	require.Nil(t, value, "credentials from the process cache must never escape")

	// An explicit invocation cache remains sufficient even without an invocation home.
	invocationCache := t.TempDir()
	writeLoginToken(t, session, invocationCache, "AKIAINVOCATION")
	env["AWS_LOGIN_CACHE_DIRECTORY"] = invocationCache
	value, err = baseCredentials(t.Context(), env)
	require.NoError(t, err)
	require.Equal(t, "AKIAINVOCATION", value.AccessKeyID)
	require.Equal(t, processHome, os.Getenv("HOME"))
	require.Equal(t, processHome, os.Getenv("USERPROFILE"))
	require.Equal(t, processCache, os.Getenv("AWS_LOGIN_CACHE_DIRECTORY"))
}

func TestExplicitStaticProfileDoesNotRequireInvocationHome(t *testing.T) {
	poisoned := poisonProcessEnvironment(t)
	home := homeWithProfiles(t, "[default]\naws_access_key_id = invocation-key\naws_secret_access_key = invocation-secret\n", "")
	value, err := baseCredentials(t.Context(), map[string]string{
		"AWS_SHARED_CREDENTIALS_FILE": filepath.Join(home, ".aws", "credentials"),
	})
	require.NoError(t, err)
	require.Equal(t, "invocation-key", value.AccessKeyID)
	require.Equal(t, "invocation-secret", value.SecretAccessKey)
	assertProcessEnvironmentUnchanged(t, poisoned)
}
