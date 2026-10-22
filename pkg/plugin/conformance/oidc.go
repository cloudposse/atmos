package conformance

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/bridge"
	"github.com/cloudposse/atmos/pkg/plugin/credential"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/plugin/testkit"
)

const (
	browserTimeoutSeconds = 30
	maxCredentialEvents   = 1024
)

type oidcRun struct {
	suite   *suite
	fixture *OIDCFixture
	issuer  *testkit.Issuer
	storage keyring.Keyring
	scope   bridge.Invocation
	spec    *structpb.Struct
	opens   atomic.Int64
}

// loopbackTransport keeps default conformance API requests on local fixtures.
type loopbackTransport struct{}

func (loopbackTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	defer perf.Track(nil, "conformance.loopbackTransport.RoundTrip")()

	ip := net.ParseIP(request.URL.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return nil, errUtils.ErrPluginAPI
	}
	return http.DefaultTransport.RoundTrip(request)
}

func (s *suite) oidc(ctx context.Context, fixture *OIDCFixture) error {
	if !s.exports("auth.provider", fixture.Provider) || !s.exports("auth.identity", fixture.Identity) {
		return withDetail(errUtils.ErrPluginInvalidConfig, "plugin.yaml must export auth.provider %q and auth.identity %q for the oidc fixture", clip(fixture.Provider), clip(fixture.Identity))
	}
	issuer, err := testkit.NewIssuer()
	if err != nil {
		return err
	}
	defer issuer.Close()
	storage, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	if err != nil {
		return err
	}
	spec, err := structpb.NewStruct(map[string]any{"issuer": issuer.Server.URL, "client_id": "conformance-client", "scopes": []any{"openid", "profile"}})
	if err != nil {
		return err
	}
	run := &oidcRun{suite: s, fixture: fixture, issuer: issuer, storage: storage, spec: spec}
	run.scope = bridge.Invocation{Namespace: "conformance/provider", HTTP: api.HTTPService{Transport: loopbackTransport{}}, Keyring: storage}
	return run.lifecycle(ctx)
}

func (run *oidcRun) lifecycle(ctx context.Context) error {
	// Fresh noninteractive login must fail before opening a browser.
	if _, err := run.authenticate(ctx); status.Code(err) != codes.FailedPrecondition {
		return withDetail(errUtils.ErrTestsFailed, "a non-interactive login without a session must fail with FailedPrecondition before opening a browser; observed status %s", status.Code(err))
	}
	run.scope.Interactive = true
	run.scope.Browser = api.LocalBrowser{Interactive: true, OpenURL: run.openBrowser}
	first, err := run.authenticate(ctx)
	if err != nil {
		return withDetail(err, "interactive PKCE login failed: %s", clip(err.Error()))
	}
	if err := run.broker(ctx, first); err != nil {
		return err
	}
	run.scope.Interactive = false
	second, err := run.authenticate(ctx)
	if err != nil {
		return withDetail(err, "cached login failed: %s", clip(err.Error()))
	}
	if token(first) != token(second) || run.opens.Load() != 1 {
		return withDetail(errUtils.ErrTestsFailed, "a cached session must return the same token without reopening the browser; same token: %t, browser opens: %d (expected 1)", token(first) == token(second), run.opens.Load())
	}
	return run.refreshAndLogout(ctx, first)
}

func (r *oidcRun) openBrowser(target string) error {
	r.opens.Add(1)
	client := &http.Client{Transport: loopbackTransport{}, Timeout: browserTimeoutSeconds * time.Second}
	response, err := client.Get(target)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	return err
}

func (r *oidcRun) authenticate(ctx context.Context) (*pluginv1.Credentials, error) {
	handle, err := r.suite.start(ctx)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	var result *pluginv1.Credentials
	err = handle.Invoke(ctx, r.scope, func(ctx context.Context, invocation *pluginv1.Invocation) error {
		stream, err := handle.Provider.Authenticate(ctx, &pluginv1.ProviderRequest{Invocation: invocation, Name: "corporate", Kind: r.fixture.Provider, Spec: r.spec, Realm: "conformance"})
		if err != nil {
			return err
		}
		result, err = receiveCredentials(stream)
		if err != nil {
			return err
		}
		_, err = credential.New().Decode(result, []string{"oidc/v1"})
		return err
	})
	return result, err
}

func receiveCredentials(stream grpc.ServerStreamingClient[pluginv1.AuthenticateEvent]) (*pluginv1.Credentials, error) {
	for range maxCredentialEvents {
		event, err := stream.Recv()
		if err != nil {
			return nil, err
		}
		if result := event.GetResult(); result != nil {
			if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
				return nil, errUtils.ErrPluginProtocol
			}
			return result, nil
		}
	}
	return nil, errUtils.ErrPluginProtocol
}

func token(credentials *pluginv1.Credentials) string {
	return credentials.GetData().GetFields()["access_token"].GetStringValue()
}

func (r *oidcRun) broker(ctx context.Context, upstream *pluginv1.Credentials) error {
	var exchanges atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer "+token(upstream) {
			http.Error(w, "invalid request", http.StatusUnauthorized)
			return
		}
		exchanges.Add(1)
		writeJSON(w, map[string]any{"access_key_id": "fixture-access", "secret_access_key": "fixture-secret", "session_token": "fixture-session", "expiration": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "region": "us-east-1"})
	}))
	defer server.Close()
	principal, err := structpb.NewStruct(map[string]any{"endpoint": server.URL})
	if err != nil {
		return err
	}
	// Recreate the native process to prove durable broker reuse.
	for range 2 {
		if err := r.exchange(ctx, principal, upstream); err != nil {
			return withDetail(err, "identity credential exchange failed: %s", clip(err.Error()))
		}
	}
	if exchanges.Load() != 1 {
		return withDetail(errUtils.ErrTestsFailed, "the broker must reuse its cached credentials across process restarts; upstream exchanges: %d (expected 1)", exchanges.Load())
	}
	return nil
}

func (r *oidcRun) exchange(ctx context.Context, principal *structpb.Struct, upstream *pluginv1.Credentials) error {
	handle, err := r.suite.start(ctx)
	if err != nil {
		return err
	}
	defer handle.Close()
	scope := r.scope
	scope.Namespace = "conformance/broker"
	return handle.Invoke(ctx, scope, func(ctx context.Context, invocation *pluginv1.Invocation) error {
		stream, err := handle.Identity.Authenticate(ctx, &pluginv1.IdentityAuthenticateRequest{Identity: &pluginv1.IdentityRequest{Invocation: invocation, Name: "broker", Kind: r.fixture.Identity, Principal: principal}, Upstream: upstream})
		if err != nil {
			return err
		}
		result, err := receiveCredentials(stream)
		if err != nil {
			return err
		}
		_, err = credential.New().Decode(result, []string{"aws/v1"})
		return err
	})
}

func (r *oidcRun) logout(ctx context.Context) error {
	handle, err := r.suite.start(ctx)
	if err != nil {
		return err
	}
	defer handle.Close()
	err = handle.Invoke(ctx, r.scope, func(ctx context.Context, invocation *pluginv1.Invocation) error {
		_, err := handle.Provider.Logout(ctx, &pluginv1.ProviderRequest{Invocation: invocation, Name: "corporate", Kind: r.fixture.Provider, Spec: r.spec})
		return err
	})
	if err != nil {
		return err
	}
	scope := r.scope
	scope.Namespace = "conformance/broker"
	return handle.Invoke(ctx, scope, func(ctx context.Context, invocation *pluginv1.Invocation) error {
		_, err := handle.Identity.Logout(ctx, &pluginv1.IdentityRequest{Invocation: invocation, Name: "broker", Kind: r.fixture.Identity})
		return err
	})
}

func writeJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

func (run *oidcRun) refreshAndLogout(ctx context.Context, first *pluginv1.Credentials) error {
	if err := testkit.ExpireOIDCSession(run.storage); err != nil {
		return err
	}
	refreshed, err := run.authenticate(ctx)
	if err != nil {
		return withDetail(err, "login with an expired session failed: %s", clip(err.Error()))
	}
	if token(first) == token(refreshed) || run.opens.Load() != 1 {
		return withDetail(errUtils.ErrTestsFailed, "an expired session must refresh to a new token without reopening the browser; token changed: %t, browser opens: %d (expected 1)", token(first) != token(refreshed), run.opens.Load())
	}
	if err := run.logout(ctx); err != nil {
		return withDetail(err, "logout failed: %s", clip(err.Error()))
	}
	if _, err := run.authenticate(ctx); status.Code(err) != codes.FailedPrecondition {
		return withDetail(errUtils.ErrTestsFailed, "login after logout must fail with FailedPrecondition; observed status %s", status.Code(err))
	}
	return run.checkLoggedOut()
}

func (run *oidcRun) checkLoggedOut() error {
	exchanges, refreshes, revocations := run.issuer.Counts()
	if exchanges != 1 || refreshes != 1 || revocations != 1 {
		return withDetail(errUtils.ErrTestsFailed, "the identity provider must see exactly one code exchange, one refresh, and one revocation; observed exchanges: %d, refreshes: %d, revocations: %d", exchanges, refreshes, revocations)
	}
	keys, err := run.storage.List()
	if err != nil {
		return err
	}
	if len(keys) != 0 {
		return withDetail(errUtils.ErrTestsFailed, "logout must remove every stored session; %d keyring entries remain", len(keys))
	}

	return nil
}
