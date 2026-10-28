package scaffold

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/cloudposse/atmos/pkg/api"
	authregistry "github.com/cloudposse/atmos/pkg/auth/registry"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/auth"
	"github.com/cloudposse/atmos/pkg/plugin/bridge"
	"github.com/cloudposse/atmos/pkg/plugin/credential"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/plugin/runtime/native"
	"github.com/cloudposse/atmos/pkg/plugin/testkit"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGeneratedOIDCNativeLoginReuseRefreshLogout(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	require.True(t, ok)
	sdk := filepath.Clean(filepath.Join(filepath.Dir(current), "..", "..", "..", "sdk"))
	project := filepath.Join(t.TempDir(), "oidc")
	require.NoError(t, Create(t.Context(), project, Options{Name: "example/oidc", Module: "example.com/oidc", SDKVersion: "v0.0.0", SDKPath: sdk, Runtime: "go", Capability: "auth.provider", Defaults: true}))
	executable := filepath.Join(project, "oidc-plugin")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	build := exec.CommandContext(t.Context(), "go", "build", "-o", executable, ".")
	build.Dir = project
	// Workspace mode resolves the unreleased SDK from the explicitly generated go.work.
	output, err := build.CombinedOutput()
	if err != nil {
		for _, name := range []string{"go.mod", "go.work"} {
			raw, readErr := os.ReadFile(filepath.Join(project, name))
			require.NoError(t, readErr)
			t.Logf("%s: %s", name, raw)
		}
	}
	require.NoError(t, err, string(output))
	verifyAuthAdapters(t, executable)
	verifyInvalidIDTokens(t, executable)
	verifyInteractiveRefreshRecovery(t, executable)
	issuer, err := testkit.NewIssuer()
	require.NoError(t, err)
	defer issuer.Close()
	storage, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	opens := 0
	scope := bridge.Invocation{
		Namespace: "example/oidc/provider/realm/fingerprint", Interactive: true, HTTP: api.HTTPService{}, Keyring: storage,
		Browser: api.LocalBrowser{Interactive: true, OpenURL: func(target string) error {
			opens++
			response, err := http.Get(target)
			if err != nil {
				return err
			}
			defer response.Body.Close()
			_, err = io.Copy(io.Discard, response.Body)
			return err
		}},
	}
	spec, err := structpb.NewStruct(map[string]any{"issuer": issuer.Server.URL, "client_id": "test-client", "scopes": []any{"openid", "profile"}})
	require.NoError(t, err)
	authenticate := func() *pluginv1.Credentials {
		handle, err := native.Start(t.Context(), executable)
		require.NoError(t, err)
		defer handle.Close()
		var result *pluginv1.Credentials
		err = handle.Invoke(t.Context(), scope, func(ctx context.Context, invocation *pluginv1.Invocation) error {
			stream, err := handle.Provider.Authenticate(ctx, &pluginv1.ProviderRequest{Invocation: invocation, Kind: "example/oidc", Name: "corporate", Spec: spec})
			if err != nil {
				return err
			}
			event, err := stream.Recv()
			if err != nil {
				return err
			}
			result = event.GetResult()
			_, err = stream.Recv()
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		return result
	}
	first := authenticate()
	assert.Equal(t, "oidc/v1", first.GetType())
	assert.Equal(t, "test-subject", first.GetData().GetFields()["subject"].GetStringValue())
	assert.NotContains(t, first.GetData().AsMap(), "refresh_token")
	verifyDownstreamChain(t, executable, scope, first)
	second := authenticate()
	assert.Equal(t, first.GetData().GetFields()["access_token"].GetStringValue(), second.GetData().GetFields()["access_token"].GetStringValue())
	assert.Equal(t, 1, opens)
	// Expire only the access token in the private fixture keyring. A new plugin
	// process must recover the retained refresh token and persist its rotation.
	require.NoError(t, testkit.ExpireOIDCSession(storage))
	renewed := authenticate()
	assert.NotEqual(t, first.GetData().GetFields()["access_token"].GetStringValue(), renewed.GetData().GetFields()["access_token"].GetStringValue())
	assert.Equal(t, 1, opens)
	handle, err := native.Start(t.Context(), executable)
	require.NoError(t, err)
	defer handle.Close()
	require.NoError(t, handle.Invoke(t.Context(), scope, func(ctx context.Context, invocation *pluginv1.Invocation) error {
		_, err := handle.Provider.Logout(ctx, &pluginv1.ProviderRequest{Invocation: invocation, Kind: "example/oidc", Name: "corporate", Spec: spec})
		return err
	}))
	require.NoError(t, handle.Invoke(t.Context(), scope, func(ctx context.Context, invocation *pluginv1.Invocation) error {
		_, err := handle.Identity.Logout(ctx, &pluginv1.IdentityRequest{Invocation: invocation, Kind: "example/oidc-broker", Name: "broker"})
		return err
	}))
	keys, err := storage.List()
	require.NoError(t, err)
	assert.Empty(t, keys)
	exchanges, refreshes, revocations := issuer.Counts()
	assert.Equal(t, 1, exchanges)
	assert.Equal(t, 1, refreshes)
	assert.Equal(t, 1, revocations)
}

//nolint:gocritic // Copy the invocation scope so request context changes cannot alias another call.
func verifyDownstreamChain(t *testing.T, executable string, scope bridge.Invocation, upstream *pluginv1.Credentials) {
	t.Helper()
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "Bearer "+upstream.GetData().GetFields()["access_token"].GetStringValue(), r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_key_id": "broker-access", "secret_access_key": "broker-secret", "session_token": "broker-session", "expiration": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "region": "us-east-1"})
	}))
	defer broker.Close()
	handle, err := native.Start(t.Context(), executable)
	require.NoError(t, err)
	defer handle.Close()
	principal, err := structpb.NewStruct(map[string]any{"endpoint": broker.URL})
	require.NoError(t, err)
	var downstream *pluginv1.Credentials
	require.NoError(t, handle.Invoke(t.Context(), scope, func(ctx context.Context, invocation *pluginv1.Invocation) error {
		stream, err := handle.Identity.Authenticate(ctx, &pluginv1.IdentityAuthenticateRequest{Identity: &pluginv1.IdentityRequest{Invocation: invocation, Name: "broker", Kind: "example/oidc-broker", Principal: principal}, Upstream: upstream})
		if err != nil {
			return err
		}
		event, err := stream.Recv()
		if err != nil {
			return err
		}
		downstream = event.GetResult()
		return nil
	}))
	decoded, err := credential.New().Decode(downstream, []string{"aws/v1"})
	require.NoError(t, err)
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, strings.Contains(r.Header.Get("Authorization"), "Credential=broker-access/"))
		assert.Equal(t, "broker-session", r.Header.Get("X-Amz-Security-Token"))
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleResult><Credentials><AccessKeyId>assumed-access</AccessKeyId><SecretAccessKey>assumed-secret</SecretAccessKey><SessionToken>assumed-session</SessionToken><Expiration>`+time.Now().Add(time.Hour).UTC().Format(time.RFC3339)+`</Expiration></Credentials><AssumedRoleUser><Arn>arn:aws:sts::123456789012:assumed-role/test/session</Arn><AssumedRoleId>test:session</AssumedRoleId></AssumedRoleUser></AssumeRoleResult></AssumeRoleResponse>`)
	}))
	defer cloud.Close()
	identity, err := authregistry.NewIdentity("downstream", &schema.Identity{Kind: "aws/assume-role", Principal: map[string]any{"assume_role": "arn:aws:iam::123456789012:role/test"}, Spec: map[string]any{"endpoint_url": cloud.URL}})
	require.NoError(t, err)
	require.NoError(t, identity.Validate())
	result, err := identity.Authenticate(t.Context(), decoded)
	require.NoError(t, err)
	require.IsType(t, &types.AWSCredentials{}, result)
	assert.Equal(t, "assumed-access", result.(*types.AWSCredentials).AccessKeyID)
}

func verifyInvalidIDTokens(t *testing.T, executable string) {
	t.Helper()
	cases := []struct {
		name   string
		option testkit.IssuerOption
	}{
		{"issuer", testkit.WithClaims(func(claims map[string]any) { claims["iss"] = "https://wrong.invalid" })},
		{"audience", testkit.WithClaims(func(claims map[string]any) { claims["aud"] = "wrong-client" })},
		{"nonce", testkit.WithClaims(func(claims map[string]any) { claims["nonce"] = "wrong-nonce" })},
		{"expired", testkit.WithClaims(func(claims map[string]any) { claims["exp"] = time.Now().Add(-time.Hour).Unix() })},
		{"authorized party", testkit.WithClaims(func(claims map[string]any) { claims["azp"] = "other-client" })},
		{"multiple audiences", testkit.WithClaims(func(claims map[string]any) { claims["aud"] = []string{"test-client", "other-client"} })},
		{"access token hash", testkit.WithClaims(func(claims map[string]any) { claims["at_hash"] = "wrong-hash" })},
		{"signature", testkit.WithInvalidSignature()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issuer, err := testkit.NewIssuer(tc.option)
			require.NoError(t, err)
			defer issuer.Close()
			storage, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
			require.NoError(t, err)
			spec, err := structpb.NewStruct(map[string]any{"issuer": issuer.Server.URL, "client_id": "test-client"})
			require.NoError(t, err)
			scope := bridge.Invocation{Namespace: "invalid-id-token", Interactive: true, HTTP: api.HTTPService{}, Keyring: storage, Browser: api.LocalBrowser{Interactive: true, OpenURL: func(target string) error {
				response, err := http.Get(target)
				if err != nil {
					return err
				}
				defer response.Body.Close()
				_, err = io.Copy(io.Discard, response.Body)
				return err
			}}}
			handle, err := native.Start(t.Context(), executable)
			require.NoError(t, err)
			defer handle.Close()
			err = handle.Invoke(t.Context(), scope, func(ctx context.Context, invocation *pluginv1.Invocation) error {
				stream, err := handle.Provider.Authenticate(ctx, &pluginv1.ProviderRequest{Invocation: invocation, Kind: "example/oidc", Name: "invalid", Spec: spec})
				if err != nil {
					return err
				}
				event, err := stream.Recv()
				assert.Nil(t, event, "invalid tokens cannot escape as credentials")
				return err
			})
			require.Equal(t, codes.Unauthenticated, status.Code(err), "invalid-token authentication returned: %v", err)
			keys, err := storage.List()
			require.NoError(t, err)
			assert.Empty(t, keys, "invalid sessions must never be persisted")
		})
	}
}

func verifyAuthAdapters(t *testing.T, executable string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	issuer, err := testkit.NewIssuer()
	require.NoError(t, err)
	defer issuer.Close()
	storage, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	interactive := true
	opens := 0
	services := adapter.Services{
		Keyring: storage, LockDirectory: t.TempDir(), Interactive: func() bool { return interactive },
		Browser: api.LocalBrowser{Interactive: true, OpenURL: func(target string) error {
			opens++
			response, err := http.Get(target)
			if err != nil {
				return err
			}
			defer response.Body.Close()
			_, err = io.Copy(io.Discard, response.Body)
			return err
		}},
	}
	definition := &adapter.Definition{
		Package: "example/oidc", Executable: executable, Outputs: []string{"oidc/v1"},
		Services: services, Credentials: credential.New(),
	}
	provider, err := adapter.ProviderConstructor(definition)("corporate", &schema.Provider{
		Kind: "example/oidc", Spec: map[string]any{"issuer": issuer.Server.URL, "client_id": "test-client"},
	})
	require.NoError(t, err)
	provider.SetRealm("adapter-test")
	first, err := provider.Authenticate(t.Context())
	require.NoError(t, err)
	upstream, ok := first.(*types.PluginCredentials)
	require.True(t, ok)
	require.Equal(t, "test-subject", upstream.Data["subject"])
	interactive = false
	second, err := provider.Authenticate(t.Context())
	require.NoError(t, err)
	require.Equal(t, first, second, "the adapter must preserve cached credential reuse across native processes")
	require.Equal(t, 1, opens)
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "Bearer "+upstream.Data["access_token"].(string), r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_key_id": "adapter-access", "secret_access_key": "adapter-secret", "session_token": "adapter-session",
			"expiration": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "region": "us-east-1",
		})
	}))
	defer broker.Close()
	identityDefinition := *definition
	identityDefinition.Outputs = []string{"aws/v1"}
	identity, err := adapter.IdentityConstructor(&identityDefinition, []string{"oidc/v1"}, adapter.AWSMaterializer)("broker", &schema.Identity{
		Kind: "example/oidc-broker", Via: &schema.IdentityVia{Provider: "corporate"}, Principal: map[string]any{"endpoint": broker.URL},
	})
	require.NoError(t, err)
	identity.SetRealm("adapter-test")
	cloud, err := identity.Authenticate(t.Context(), first)
	require.NoError(t, err)
	require.IsType(t, &types.AWSCredentials{}, cloud)
	require.Equal(t, "adapter-access", cloud.(*types.AWSCredentials).AccessKeyID)
	configuration := &schema.AuthContext{}
	require.NoError(t, identity.PostAuthenticate(t.Context(), &types.PostAuthenticateParams{
		ProviderName: "corporate", Realm: "adapter-test", Credentials: cloud, AuthContext: configuration, StackInfo: &schema.ConfigAndStacksInfo{},
	}))
	environment, err := identity.Environment()
	require.NoError(t, err)
	require.Equal(t, "broker", environment["AWS_PROFILE"])
	require.NoError(t, identity.Logout(t.Context()))
	require.NoError(t, provider.Logout(t.Context()))
	keys, err := storage.List()
	require.NoError(t, err)
	require.Empty(t, keys, "both adapters must delete their private session after logout")
}

func verifyInteractiveRefreshRecovery(t *testing.T, executable string) {
	t.Helper()
	for _, refresh := range []string{"", "revoked-refresh-token"} {
		t.Run("refresh recovery/"+refresh, func(t *testing.T) {
			issuer, err := testkit.NewIssuer()
			require.NoError(t, err)
			t.Cleanup(issuer.Close)
			storage, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
			require.NoError(t, err)
			opens := 0
			scope := bridge.Invocation{
				Namespace: "refresh-recovery", Interactive: true, HTTP: api.HTTPService{}, Keyring: storage,
				Browser: api.LocalBrowser{Interactive: true, OpenURL: func(target string) error {
					opens++
					response, err := http.Get(target)
					if err != nil {
						return err
					}
					defer response.Body.Close()
					_, err = io.Copy(io.Discard, response.Body)
					return err
				}},
			}
			spec, err := structpb.NewStruct(map[string]any{"issuer": issuer.Server.URL, "client_id": "test-client"})
			require.NoError(t, err)
			authenticate := func() (*pluginv1.Credentials, error) {
				handle, err := native.Start(t.Context(), executable)
				if err != nil {
					return nil, err
				}
				defer handle.Close()
				var result *pluginv1.Credentials
				err = handle.Invoke(t.Context(), scope, func(ctx context.Context, invocation *pluginv1.Invocation) error {
					stream, err := handle.Provider.Authenticate(ctx, &pluginv1.ProviderRequest{Invocation: invocation, Kind: "example/oidc", Name: "corporate", Spec: spec})
					if err != nil {
						return err
					}
					event, err := stream.Recv()
					if err != nil {
						return err
					}
					result = event.GetResult()
					return nil
				})
				return result, err
			}
			original, err := authenticate()
			require.NoError(t, err)
			keys, err := storage.List()
			require.NoError(t, err)
			require.Len(t, keys, 1)
			encoded, err := storage.Get(keys[0])
			require.NoError(t, err)
			raw, err := base64.StdEncoding.DecodeString(encoded)
			require.NoError(t, err)
			var envelope struct {
				Version int               `json:"version"`
				Values  map[string][]byte `json:"values"`
			}
			require.NoError(t, json.Unmarshal(raw, &envelope))
			var session map[string]any
			require.NoError(t, json.Unmarshal(envelope.Values["session/v1"], &session))
			token := session["token"].(map[string]any)
			token["expiry"] = time.Unix(1, 0).UTC().Format(time.RFC3339)
			token["refresh_token"] = refresh
			envelope.Values["session/v1"], err = json.Marshal(session)
			require.NoError(t, err)
			raw, err = json.Marshal(envelope)
			require.NoError(t, err)
			expired := base64.StdEncoding.EncodeToString(raw)
			require.NoError(t, storage.Set(keys[0], expired))
			scope.Interactive = false
			result, err := authenticate()
			require.Equal(t, codes.FailedPrecondition, status.Code(err))
			require.Nil(t, result)
			require.Equal(t, 1, opens, "noninteractive refresh failure must not launch a browser")
			// Restore the same expired session: interactive recovery must succeed on its
			// first invocation, even when invalid_grant previously removed the session.
			require.NoError(t, storage.Set(keys[0], expired))
			scope.Interactive = true
			recovered, err := authenticate()
			require.NoError(t, err)
			require.NotEqual(t, original.GetData().GetFields()["access_token"].GetStringValue(), recovered.GetData().GetFields()["access_token"].GetStringValue())
			require.Equal(t, 2, opens)
			scope.Interactive = false
			cached, err := authenticate()
			require.NoError(t, err)
			require.Equal(t, recovered.GetData().AsMap(), cached.GetData().AsMap(), "fresh login must replace the unusable session durably")
			require.Equal(t, 2, opens)
		})
	}
}
