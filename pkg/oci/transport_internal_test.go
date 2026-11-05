package oci

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/errcode"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/keyring"
)

type failingAuthenticator struct{}

func (failingAuthenticator) Authorization() (*authn.AuthConfig, error) { return nil, os.ErrPermission }

func TestCredentialFromAdaptsEveryAuthenticatorShape(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("encoded-user:encoded-secret"))
	for _, tc := range []struct {
		name          string
		authenticator authn.Authenticator
		want          auth.Credential
		err           error
	}{
		{"nil authenticator is anonymous", nil, auth.EmptyCredential, nil},
		{"anonymous", authn.Anonymous, auth.EmptyCredential, nil},
		{"basic", &authn.Basic{Username: "u", Password: "p"}, auth.Credential{Username: "u", Password: "p"}, nil},
		{"registry token", &authn.Bearer{Token: "access"}, auth.Credential{AccessToken: "access"}, nil},
		{"identity token", authn.FromConfig(authn.AuthConfig{IdentityToken: "refresh"}), auth.Credential{RefreshToken: "refresh"}, nil},
		{"base64 auth field", authn.FromConfig(authn.AuthConfig{Auth: encoded}), auth.Credential{Username: "encoded-user", Password: "encoded-secret"}, nil},
		{"explicit fields win over auth field", authn.FromConfig(authn.AuthConfig{Username: "u", Password: "p", Auth: encoded}), auth.Credential{Username: "u", Password: "p"}, nil},
		{"malformed auth field", authn.FromConfig(authn.AuthConfig{Auth: "!!"}), auth.EmptyCredential, errUtils.ErrOCIRegistry},
		{"auth field without separator", authn.FromConfig(authn.AuthConfig{Auth: base64.StdEncoding.EncodeToString([]byte("nocolon"))}), auth.EmptyCredential, errUtils.ErrOCIRegistry},
		{"authenticator failure fails closed", failingAuthenticator{}, auth.EmptyCredential, os.ErrPermission},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := credentialFrom(tc.authenticator)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				assert.Equal(t, auth.EmptyCredential, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCredentialsAreReleasedOnlyToTheirRegistry(t *testing.T) {
	credential := hostScopedCredential("registry.example.com:5000", &authn.Basic{Username: "u", Password: "p"})
	for host, wantSecret := range map[string]bool{
		"registry.example.com:5000": true,
		"REGISTRY.example.com:5000": true,
		"registry.example.com":      false,
		"evil.example.com:5000":     false,
		"auth.example.com":          false,
	} {
		got, err := credential(t.Context(), host)
		require.NoError(t, err)
		assert.Equal(t, wantSecret, got.Password == "p", host)
	}

	hub := hostScopedCredential("docker.io", &authn.Basic{Username: "u", Password: "p"})
	got, err := hub(t.Context(), "registry-1.docker.io")
	require.NoError(t, err)
	assert.Equal(t, "p", got.Password, "Docker Hub traffic is redirected to its API endpoint")
	assert.Equal(t, "docker.io", orasHost("index.docker.io"))
	assert.Equal(t, "ghcr.io", orasHost("ghcr.io"))
}

func TestRepositoryFailsClosedBeforeAnyRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	repository := Repository{Registry: host, Path: "acme/plugin"}
	resource, err := repository.legacy()
	require.NoError(t, err)

	for _, tc := range []struct {
		name    string
		auth    authn.Authenticator
		matched bool
		err     error
	}{
		{"resolver error", nil, true, os.ErrPermission},
		{"matched identity without credentials", nil, true, nil},
		{"matched identity resolves to anonymous", authn.Anonymous, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controller := gomock.NewController(t)
			resolver := NewMockAuthenticationResolver(controller)
			resolver.EXPECT().Resolve(gomock.Any(), resource).Return(tc.auth, tc.matched, tc.err)
			// Neither ambient keyring credentials nor anonymous access may stand in for the identity.
			store := keyring.NewMockKeyring(controller)
			client := NewClient(nil, WithAuthenticationResolver(resolver), WithKeychain(CredentialKeychain{Store: store}))
			opened, err := client.Repository(t.Context(), repository)
			require.ErrorIs(t, err, errUtils.ErrOCIRegistry)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			}
			assert.Nil(t, opened)
		})
	}
	assert.Zero(t, requests.Load(), "no request may reach the registry when the identity cannot be resolved")
}

func TestBoundIdentityTakesPrecedenceOverAmbientCredentials(t *testing.T) {
	var seen atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, password, ok := r.BasicAuth(); ok {
			seen.Store(user + ":" + password)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"acme/plugin","tags":["1.0.0"]}`))
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	repository := Repository{Registry: host, Path: "acme/plugin"}
	resource, err := repository.legacy()
	require.NoError(t, err)

	controller := gomock.NewController(t)
	resolver := NewMockAuthenticationResolver(controller)
	resolver.EXPECT().Resolve(gomock.Any(), resource).Return(&authn.Basic{Username: "identity", Password: "bound"}, true, nil)
	store, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	chain := CredentialKeychain{Store: store}
	require.NoError(t, chain.Login(host, &authn.AuthConfig{Username: "ambient", Password: "stored"}))

	client := NewClient(nil, WithAuthenticationResolver(resolver), WithKeychain(chain))
	opened, err := client.Repository(t.Context(), repository)
	require.NoError(t, err)
	tags, err := opened.Tags(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"1.0.0"}, tags)
	assert.Equal(t, "identity:bound", seen.Load())
}

func TestParseRepositoryUsesLegacyRegistryRules(t *testing.T) {
	repository, err := ParseRepository("ghcr.io/acme/plugin")
	require.NoError(t, err)
	assert.Equal(t, Repository{Registry: "ghcr.io", Path: "acme/plugin"}, repository)
	assert.Equal(t, "ghcr.io/acme/plugin", repository.String())

	hub, err := ParseRepository("library/nginx")
	require.NoError(t, err)
	assert.Equal(t, Repository{Registry: "index.docker.io", Path: "library/nginx"}, hub)

	_, err = ParseRepository("not a repository")
	require.ErrorIs(t, err, errUtils.ErrOCIRegistry)
	_, err = Repository{Registry: "bad registry", Path: "x"}.legacy()
	require.ErrorIs(t, err, errUtils.ErrOCIRegistry)
}

func TestNewReferrersIndexRoundTripsDescriptors(t *testing.T) {
	found := []ocispec.Descriptor{{
		MediaType: ocispec.MediaTypeImageManifest, Size: 10, Digest: digest.FromString("a"),
		ArtifactType: "application/vnd.dev.sigstore.bundle.v0.3+json", Annotations: map[string]string{"k": "v"},
	}}
	index, err := newReferrersIndex(found)
	require.NoError(t, err)
	manifest, err := index.IndexManifest()
	require.NoError(t, err)
	require.Len(t, manifest.Manifests, 1)
	assert.Equal(t, found[0].Digest.String(), manifest.Manifests[0].Digest.String())
	assert.Equal(t, found[0].ArtifactType, manifest.Manifests[0].ArtifactType)
	assert.Equal(t, found[0].Annotations, manifest.Manifests[0].Annotations)
	size, err := index.Size()
	require.NoError(t, err)
	raw, err := index.RawManifest()
	require.NoError(t, err)
	assert.Equal(t, int64(len(raw)), size)
	hash, err := index.Digest()
	require.NoError(t, err)
	assert.Equal(t, digest.FromBytes(raw).String(), hash.String())
	_, err = index.Image(hash)
	require.ErrorIs(t, err, errUtils.ErrOCIRegistry)
	_, err = index.ImageIndex(hash)
	require.ErrorIs(t, err, errUtils.ErrOCIRegistry)

	_, err = newReferrersIndex([]ocispec.Descriptor{{Digest: "not-a-digest"}})
	require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
}

func TestStatusClassification(t *testing.T) {
	notFound := wrapRegistry(&errorResponse404)
	assert.True(t, IsNotFound(notFound))
	assert.Equal(t, http.StatusNotFound, statusOf(notFound))
	assert.True(t, definiteRejection(wrapRegistry(&errorResponse403)))
	assert.False(t, definiteRejection(wrapRegistry(errors.New("connection reset"))))
	assert.False(t, definiteRejection(wrapRegistry(&errorResponse503)))
	assert.False(t, IsNotFound(wrapRegistry(&errorResponse403)))
	assert.False(t, IsNotFound(nil))
	assert.NoError(t, wrapRegistry(nil))
}

var (
	errorResponse404 = errcode.ErrorResponse{Method: http.MethodGet, StatusCode: http.StatusNotFound}
	errorResponse403 = errcode.ErrorResponse{Method: http.MethodPut, StatusCode: http.StatusForbidden}
	errorResponse503 = errcode.ErrorResponse{Method: http.MethodPut, StatusCode: http.StatusServiceUnavailable}
)
