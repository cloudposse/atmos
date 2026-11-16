package oci_test

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
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

// Predecessor discovery during an extended copy uses the same fallback as Referrers, so a registry that
// answers the referrers API with 400 still yields its attachments without a prior Referrers call.
func TestExtendedCopyDiscoversAttachmentsThroughTheReferrersFallback(t *testing.T) {
	var refuse atomic.Bool
	backing := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if refuse.Load() && strings.Contains(r.URL.Path, "/referrers/") {
			w.WriteHeader(http.StatusBadRequest)
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
	require.NoError(t, store.Tag(t.Context(), subject, "1.0.0"))
	refuse.Store(true)
	fresh, err := client.Repository(t.Context(), oci.Repository{Registry: host, Path: "acme/plugin"})
	require.NoError(t, err)
	layout, err := oci.OpenLayout(t.Context(), filepath.Join(t.TempDir(), "layout"))
	require.NoError(t, err)

	_, err = oci.ExtendedCopy(t.Context(), oci.GraphSource{Store: fresh, Reference: "1.0.0"}, oci.Destination{Store: layout, Reference: "1.0.0"})

	require.NoError(t, err)
	copied, err := layout.Exists(t.Context(), attachment)
	require.NoError(t, err)
	assert.True(t, copied, "the attachment was discovered through the tag-schema fallback")
}

// Deleting an attachment needs the registry's referrers capability to keep the tag-schema index current.
// A 400 on the capability probe settles it through the fallback; the DELETE itself is sent exactly once.
func TestDeleteAttachmentSettlesTheReferrersCapabilityThroughTheFallback(t *testing.T) {
	// The registry first answers the referrers API with 404, so pushes maintain the tag-schema index, and
	// then with 400, the way a registry that rejects the endpoint does.
	var referrersStatus atomic.Int32
	referrersStatus.Store(http.StatusNotFound)
	var mu sync.Mutex
	var deletes []string
	backing := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/referrers/") {
			w.WriteHeader(int(referrersStatus.Load()))
			return
		}
		if r.Method == http.MethodDelete {
			mu.Lock()
			deletes = append(deletes, r.URL.Path)
			mu.Unlock()
		}
		backing.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	client := registryClient(t, host, nil)
	store, err := client.Repository(t.Context(), oci.Repository{Registry: host, Path: "acme/plugin"})
	require.NoError(t, err)
	_, attachment := pushAttachment(t, store)
	referrersStatus.Store(http.StatusBadRequest)
	ref, err := name.NewDigest(host + "/acme/plugin@" + attachment.Digest.String())
	require.NoError(t, err)

	require.NoError(t, client.Delete(t.Context(), ref))

	mu.Lock()
	defer mu.Unlock()
	attachmentDeletes := 0
	for _, deleted := range deletes {
		if strings.HasSuffix(deleted, "/manifests/"+attachment.Digest.String()) {
			attachmentDeletes++
		}
	}
	assert.Equal(t, 1, attachmentDeletes, "the DELETE of the attachment is sent exactly once")
	fresh, err := client.Repository(t.Context(), oci.Repository{Registry: host, Path: "acme/plugin"})
	require.NoError(t, err)
	exists, err := fresh.Exists(t.Context(), attachment)
	require.NoError(t, err)
	assert.False(t, exists, "the attachment is deleted")
}
