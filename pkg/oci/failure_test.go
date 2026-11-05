package oci

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/keyring"
)

//go:generate go run go.uber.org/mock/mockgen -destination mock_identity_test.go -package oci github.com/cloudposse/atmos/pkg/oci AuthenticationResolver

func TestBoundIdentityNeverFallsBackToAmbientCredentials(t *testing.T) {
	t.Parallel()
	resource, err := name.NewRepository("registry.example.com/acme/plugin")
	require.NoError(t, err)
	for _, tc := range []struct {
		name    string
		auth    authn.Authenticator
		matched bool
		err     error
	}{
		{name: "bound credentials", auth: authn.FromConfig(authn.AuthConfig{Username: "identity", Password: "bound-secret"}), matched: true},
		{name: "missing credentials", matched: true},
		{name: "anonymous credentials", auth: authn.Anonymous, matched: true},
		{name: "resolver failed", err: os.ErrPermission},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			controller := gomock.NewController(t)
			resolver := NewMockAuthenticationResolver(controller)
			resolver.EXPECT().Resolve(gomock.Any(), resource).Return(tc.auth, tc.matched, tc.err)
			// No keyring calls are allowed for an explicitly bound identity, even when it fails.
			store := keyring.NewMockKeyring(controller)
			client := NewClient(nil, WithAuthenticationResolver(resolver), WithKeychain(CredentialKeychain{Store: store}))
			actual, err := client.authentication(t.Context(), resource)
			if tc.name == "bound credentials" {
				require.NoError(t, err)
				credentials, err := actual.Authorization()
				require.NoError(t, err)
				require.Equal(t, "identity", credentials.Username)
				require.Equal(t, "bound-secret", credentials.Password)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrOCIRegistry)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			}
			require.Nil(t, actual)
		})
	}
}

func TestUnboundIdentityUsesHostScopedStoredCredentials(t *testing.T) {
	t.Parallel()
	resource, err := name.NewRepository("registry.example.com/acme/plugin")
	require.NoError(t, err)
	controller := gomock.NewController(t)
	resolver := NewMockAuthenticationResolver(controller)
	resolver.EXPECT().Resolve(gomock.Any(), resource).Return(nil, false, nil)
	store := keyring.NewMockKeyring(controller)
	store.EXPECT().Get(registryCredentialKey(resource.RegistryStr())).Return(`{"username":"stored","password":"stored-secret"}`, nil)
	client := NewClient(nil, WithAuthenticationResolver(resolver), WithKeychain(CredentialKeychain{Store: store}))
	actual, err := client.authentication(t.Context(), resource)
	require.NoError(t, err)
	credentials, err := actual.Authorization()
	require.NoError(t, err)
	require.Equal(t, "stored", credentials.Username)
	require.Equal(t, "stored-secret", credentials.Password)
}

func TestCredentialsRejectInvalidInputAndBackendFailures(t *testing.T) {
	resource, err := name.NewRepository("registry.example.com/acme/plugin")
	require.NoError(t, err)
	chain := CredentialKeychain{}
	_, err = chain.Resolve(resource)
	require.ErrorIs(t, err, errUtils.ErrOCIRegistry)
	require.ErrorIs(t, chain.Login("registry.example.com", &authn.AuthConfig{Password: "fixture"}), errUtils.ErrOCIRegistry)
	require.ErrorIs(t, chain.Logout("registry.example.com"), errUtils.ErrOCIRegistry)
	require.ErrorIs(t, chain.Login("bad registry", &authn.AuthConfig{Password: "fixture"}), errUtils.ErrOCIRegistry)
	require.ErrorIs(t, chain.Login("registry.example.com", nil), errUtils.ErrOCIRegistry)
	require.ErrorIs(t, chain.Login("registry.example.com", &authn.AuthConfig{Username: "user"}), errUtils.ErrOCIRegistry)
	require.ErrorIs(t, chain.Logout("bad registry"), errUtils.ErrOCIRegistry)
	store := keyring.NewMockKeyring(gomock.NewController(t))
	chain.Store = store
	key := registryCredentialKey(resource.RegistryStr())
	store.EXPECT().Get(key).Return("", os.ErrPermission)
	_, err = chain.Resolve(resource)
	require.ErrorIs(t, err, errUtils.ErrOCIRegistry)
	require.ErrorIs(t, err, os.ErrPermission)
	store.EXPECT().Get(key).Return("invalid JSON", nil)
	_, err = chain.Resolve(resource)
	require.ErrorIs(t, err, errUtils.ErrOCIRegistry)
	store.EXPECT().Set(key, gomock.Any()).Return(os.ErrPermission)
	require.ErrorIs(t, chain.Login("registry.example.com", &authn.AuthConfig{IdentityToken: "fixture"}), os.ErrPermission)
	store.EXPECT().Delete(key).Return(os.ErrPermission)
	require.ErrorIs(t, chain.Logout("registry.example.com"), os.ErrPermission)
}

func TestClientDoesNotMakeRequestsWhenCredentialsFail(t *testing.T) {
	ref, err := name.NewTag("registry.example.com/acme/plugin:1.0.0")
	require.NoError(t, err)
	failure := CredentialKeychain{Open: func() (keyring.Keyring, error) { return nil, keyring.ErrPasswordRequired }}
	for _, useFallback := range []bool{false, true} {
		chain := failure
		options := []ClientOption{WithKeychain(chain)}
		if useFallback {
			store, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
			require.NoError(t, err)
			options = []ClientOption{WithKeychain(CredentialKeychain{Store: store}), WithCredentialFallback(failure)}
		}
		client := NewClient(nil, options...)
		for operation, run := range registryOperations(client, ref) {
			t.Run(operation, func(t *testing.T) {
				err := run(t.Context())
				require.ErrorIs(t, err, keyring.ErrPasswordRequired)
				require.ErrorIs(t, err, errUtils.ErrOCIRegistry)
			})
		}
	}
}

func registryOperations(client *Client, ref name.Tag) map[string]func(context.Context) error {
	return map[string]func(context.Context) error{
		"get":         func(ctx context.Context) error { _, err := client.Get(ctx, ref); return err },
		"head":        func(ctx context.Context) error { _, err := client.Head(ctx, ref); return err },
		"tags":        func(ctx context.Context) error { _, err := client.Tags(ctx, ref.Context()); return err },
		"write image": func(ctx context.Context) error { return client.WriteImage(ctx, ref, empty.Image) },
		"write index": func(ctx context.Context) error { return client.WriteIndex(ctx, ref, empty.Index) },
		"delete": func(ctx context.Context) error {
			return client.Delete(ctx, ref.Context().Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
		},
	}
}

func TestClientPropagatesRegistryRejectionWithCustomTransport(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests++; w.WriteHeader(http.StatusForbidden) }))
	t.Cleanup(server.Close)
	ref, err := name.NewTag(server.Listener.Addr().String() + "/acme/plugin:1.0.0")
	require.NoError(t, err)
	store, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	client := NewClient(nil, WithKeychain(CredentialKeychain{Store: store}), WithTransport(server.Client().Transport))
	for operation, run := range registryOperations(client, ref) {
		t.Run(operation, func(t *testing.T) { require.Error(t, run(t.Context())) })
	}
	require.Positive(t, requests, "the configured TLS transport must reach the registry")
}

func TestExtractVerifiedTarRejectsUnsafeEntriesBeforeWriting(t *testing.T) {
	tests := map[string][]tar.Header{
		"traversal":                  {{Name: "../escape", Typeflag: tar.TypeReg, Mode: 0o644}},
		"link":                       {{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "target", Mode: 0o755}},
		"special mode":               {{Name: "file", Typeflag: tar.TypeReg, Mode: 0o600}},
		"case insensitive duplicate": {{Name: "file", Typeflag: tar.TypeReg, Mode: 0o644}, {Name: "FILE", Typeflag: tar.TypeReg, Mode: 0o644}},
	}
	for label, headers := range tests {
		t.Run(label, func(t *testing.T) {
			var raw bytes.Buffer
			writer := tar.NewWriter(&raw)
			for index := range headers {
				require.NoError(t, writer.WriteHeader(&headers[index]))
			}
			require.NoError(t, writer.Close())
			destination := t.TempDir()
			require.ErrorIs(t, ExtractTar(raw.Bytes(), destination), errUtils.ErrOCIIntegrity)
			files, err := os.ReadDir(destination)
			require.NoError(t, err)
			require.Empty(t, files)
		})
	}
	require.ErrorIs(t, ExtractTar([]byte("invalid archive"), t.TempDir()), errUtils.ErrOCIIntegrity)
}

func TestVerifiedBlobRejectsInvalidDescriptors(t *testing.T) {
	for _, descriptor := range []*v1.Descriptor{nil, {Size: -1}, {Size: 100}} {
		_, err := ReadVerifiedBlob(bytes.NewReader([]byte("data")), descriptor, 4)
		require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
	}
	_, err := ReadVerifiedBlob(bytes.NewReader([]byte("data")), &v1.Descriptor{Size: 4}, 0)
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)

	file, err := os.CreateTemp(t.TempDir(), "closed")
	require.NoError(t, err)
	require.NoError(t, file.Close())
	_, err = ReadVerifiedBlob(file, &v1.Descriptor{}, 4)
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
	require.ErrorIs(t, err, os.ErrClosed)
}

func TestIsNotFoundDoesNotHideAuthenticationFailures(t *testing.T) {
	require.True(t, IsNotFound(fmt.Errorf("manifest: %w", &transport.Error{StatusCode: http.StatusNotFound})))
	require.False(t, IsNotFound(&transport.Error{StatusCode: http.StatusUnauthorized}))
	require.False(t, IsNotFound(context.DeadlineExceeded))
	require.False(t, IsNotFound(nil))
}

func TestVerifiedBlobChecksExactSizeAndDigest(t *testing.T) {
	original := []byte("verified content")
	digest, size, err := v1.SHA256(bytes.NewReader(original))
	require.NoError(t, err)
	descriptor := &v1.Descriptor{Digest: digest, Size: size}
	actual, err := ReadVerifiedBlob(bytes.NewReader(original), descriptor, size)
	require.NoError(t, err)
	require.Equal(t, original, actual)
	for name, bytes := range map[string][]byte{"changed": []byte("modified content"), "short": original[:len(original)-1], "long": append(append([]byte{}, original...), 'x')} {
		t.Run(name, func(t *testing.T) {
			actual, err := ReadVerifiedBlob(strings.NewReader(string(bytes)), descriptor, size)
			require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
			require.Nil(t, actual)
		})
	}
}
