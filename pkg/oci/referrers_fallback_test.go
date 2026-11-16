package oci_test

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/registry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/oci/ocitest"
)

// registryClient returns a client that can present the identity for host.
func registryClient(t *testing.T, host string, credential *ocitest.Credential) *oci.Client {
	t.Helper()
	store, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	chain := oci.CredentialKeychain{Store: store}
	if credential != nil {
		require.NoError(t, chain.Login(host, &authn.AuthConfig{Username: credential.Username, Password: credential.Password}))
	}
	return oci.NewClient(nil, oci.WithKeychain(chain))
}

// pushAttachment pushes a subject manifest and one attachment that names it, and returns both descriptors.
func pushAttachment(t *testing.T, store oci.RepositoryStore) (subject, attachment ocispec.Descriptor) {
	t.Helper()
	ctx := t.Context()
	config := []byte("{}")
	configDesc := oci.Descriptor("application/vnd.oci.empty.v1+json", config)
	require.NoError(t, store.Push(ctx, configDesc, strings.NewReader(string(config))))
	push := func(subject *ocispec.Descriptor, note string) ocispec.Descriptor {
		manifest := ocispec.Manifest{Config: configDesc, Layers: []ocispec.Descriptor{}, Subject: subject, Annotations: map[string]string{"note": note}}
		manifest.SchemaVersion = 2
		manifest.MediaType = ocispec.MediaTypeImageManifest
		raw, err := json.Marshal(manifest)
		require.NoError(t, err)
		desc := oci.Descriptor(ocispec.MediaTypeImageManifest, raw)
		require.NoError(t, store.Push(ctx, desc, strings.NewReader(string(raw))))
		return desc
	}
	subject = push(nil, "subject")
	attachment = push(&subject, "attachment")
	return subject, attachment
}

func referrerDigests(referrers []ocispec.Descriptor) []string {
	digests := make([]string, 0, len(referrers))
	for _, referrer := range referrers {
		digests = append(digests, referrer.Digest.String())
	}
	return digests
}

// The fixture serves the native OCI 1.1 referrers API by default and the tag-schema fallback on request;
// attachment discovery works against both.
func TestReferrersDiscoverAttachmentsOnNativeAndFallbackRegistries(t *testing.T) {
	credential := &ocitest.Credential{Username: "publisher", Password: "s3cret"}
	for name, start := range map[string]func(*testing.T, *ocitest.Credential) *ocitest.Server{
		"native":   ocitest.NewServer,
		"fallback": ocitest.NewServerWithoutReferrers,
	} {
		t.Run(name, func(t *testing.T) {
			server := start(t, credential)
			client := registryClient(t, server.Host, credential)
			store, err := client.Repository(t.Context(), oci.Repository{Registry: server.Host, Path: "acme/plugin"})
			require.NoError(t, err)
			subject, attachment := pushAttachment(t, store)

			referrers, err := store.Referrers(t.Context(), subject, "")

			require.NoError(t, err)
			assert.Equal(t, []string{attachment.Digest.String()}, referrerDigests(referrers))
		})
	}
}

// Some registries answer the referrers API with 400 or 406 instead of 404. That means the API is not
// available, so discovery falls back to the tag schema; other statuses stay errors.
func TestReferrersFallBackToTheTagSchemaOnBadRequestAndNotAcceptable(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusNotAcceptable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var refuse atomic.Bool
			backing := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if refuse.Load() && strings.Contains(r.URL.Path, "/referrers/") {
					w.WriteHeader(status)
					return
				}
				backing.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)
			host := strings.TrimPrefix(server.URL, "http://")
			client := registryClient(t, host, nil)
			store, err := client.Repository(t.Context(), oci.Repository{Registry: host, Path: "acme/plugin"})
			require.NoError(t, err)
			subject, attachment := pushAttachment(t, store)
			refuse.Store(true)
			// A new store has not learned whether the registry supports the referrers API, as in a later process.
			fresh, err := client.Repository(t.Context(), oci.Repository{Registry: host, Path: "acme/plugin"})
			require.NoError(t, err)

			referrers, err := fresh.Referrers(t.Context(), subject, "")

			require.NoError(t, err)
			assert.Equal(t, []string{attachment.Digest.String()}, referrerDigests(referrers))
		})
	}
}

func TestReferrersKeepOtherStatusesAsErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/referrers/") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	client := registryClient(t, host, nil)
	store, err := client.Repository(t.Context(), oci.Repository{Registry: host, Path: "acme/plugin"})
	require.NoError(t, err)

	_, err = store.Referrers(t.Context(), oci.Descriptor(ocispec.MediaTypeImageManifest, []byte("x")), "")

	require.Error(t, err)
}
