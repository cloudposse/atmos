package oci_test

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci"
)

// tagSchemaRegistry is a registry without the OCI 1.1 referrers API, so attachments are indexed through
// the referrers tag schema. Manifest DELETEs answer 405 except for the digests in allowDelete, the way
// GHCR refuses to delete the replaced referrers index. Every DELETE path is recorded.
type tagSchemaRegistry struct {
	host string

	mu          sync.Mutex
	deletes     []string
	allowDelete map[string]bool
	rejectTags  bool
}

func newTagSchemaRegistry(t *testing.T) *tagSchemaRegistry {
	t.Helper()
	fixture := &tagSchemaRegistry{allowDelete: map[string]bool{}}
	backing := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/referrers/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fixture.mu.Lock()
		defer fixture.mu.Unlock()
		if r.Method == http.MethodPut && fixture.rejectTags && strings.Contains(r.URL.Path, "/manifests/sha256-") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/manifests/") {
			fixture.deletes = append(fixture.deletes, r.URL.Path)
			digest := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			if !fixture.allowDelete[digest] {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
		}
		backing.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	fixture.host = strings.TrimPrefix(server.URL, "http://")
	return fixture
}

func (f *tagSchemaRegistry) refusedIndexDeletes(allowed ...ocispec.Descriptor) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	refused := 0
	for _, path := range f.deletes {
		permitted := false
		for _, desc := range allowed {
			permitted = permitted || strings.HasSuffix(path, "/manifests/"+desc.Digest.String())
		}
		if !permitted {
			refused++
		}
	}
	return refused
}

// attachmentManifest builds a manifest that names subject, distinguished by note. It uses the same empty
// config pushAttachment pushed, so the config blob already exists in the repository.
func attachmentManifest(t *testing.T, subject *ocispec.Descriptor, note string) (ocispec.Descriptor, []byte) {
	t.Helper()
	config := oci.Descriptor("application/vnd.oci.empty.v1+json", []byte("{}"))
	manifest := ocispec.Manifest{Config: config, Layers: []ocispec.Descriptor{}, Subject: subject, Annotations: map[string]string{"note": note}}
	manifest.SchemaVersion = 2
	manifest.MediaType = ocispec.MediaTypeImageManifest
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	return oci.Descriptor(ocispec.MediaTypeImageManifest, raw), raw
}

// Re-signing a release on a registry without the referrers API replaces the tag-schema referrers index.
// The new index is pushed and tagged before the replaced one is deleted, so a registry that refuses that
// DELETE (GHCR answers 405) has already recorded the attachment: the push succeeds (#3760).
func TestPushAttachmentSucceedsWhenTheRegistryRefusesToDeleteTheReplacedReferrersIndex(t *testing.T) {
	fixture := newTagSchemaRegistry(t)
	client := registryClient(t, fixture.host, nil)
	store, err := client.Repository(t.Context(), oci.Repository{Registry: fixture.host, Path: "acme/plugin"})
	require.NoError(t, err)
	// The first attachment creates the referrers index, so there is nothing to delete yet.
	subject, first := pushAttachment(t, store)

	byDigest, byDigestRaw := attachmentManifest(t, &subject, "signature by digest")
	pushErr := store.Push(t.Context(), byDigest, strings.NewReader(string(byDigestRaw)))
	byTag, byTagRaw := attachmentManifest(t, &subject, "signature by reference")
	pushReferenceErr := store.PushReference(t.Context(), byTag, strings.NewReader(string(byTagRaw)), byTag.Digest.String())

	require.NoError(t, pushErr, "Push of an attachment")
	require.NoError(t, pushReferenceErr, "PushReference of an attachment")
	assert.Equal(t, 2, fixture.refusedIndexDeletes(), "each later attachment asked to delete the replaced index and was refused")
	fresh, err := client.Repository(t.Context(), oci.Repository{Registry: fixture.host, Path: "acme/plugin"})
	require.NoError(t, err)
	referrers, err := fresh.Referrers(t.Context(), subject, "")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{first.Digest.String(), byDigest.Digest.String(), byTag.Digest.String()}, referrerDigests(referrers),
		"every attachment is listed by the new referrers index")
}

// Only the cleanup of the replaced index is forgiven. When the registry refuses the new referrers index
// itself, the attachment is not discoverable and the push still fails.
func TestPushAttachmentFailsWhenTheRegistryRefusesTheNewReferrersIndex(t *testing.T) {
	fixture := newTagSchemaRegistry(t)
	client := registryClient(t, fixture.host, nil)
	store, err := client.Repository(t.Context(), oci.Repository{Registry: fixture.host, Path: "acme/plugin"})
	require.NoError(t, err)
	subject, _ := pushAttachment(t, store)
	fixture.mu.Lock()
	fixture.rejectTags = true
	fixture.mu.Unlock()

	desc, raw := attachmentManifest(t, &subject, "signature")
	err = store.Push(t.Context(), desc, strings.NewReader(string(raw)))

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrOCIRegistry)
	assert.Equal(t, 0, fixture.refusedIndexDeletes(), "the replaced index is never deleted when the new one was not tagged")
}

// Deletes are not forgiven. ORAS replaces the referrers index before it deletes the target, so when the
// registry refuses to delete the replaced index, the target DELETE is never sent and the attachment stays
// downloadable. Removing the last attachment pushes no replacement index at all, so it also stays listed.
// Reporting success in either case would claim a removal that did not happen.
func TestDeleteAttachmentFailsWhenTheRegistryRefusesToDeleteTheReplacedReferrersIndex(t *testing.T) {
	for scenario, extra := range map[string]bool{"one of several attachments": true, "the last attachment": false} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newTagSchemaRegistry(t)
			client := registryClient(t, fixture.host, nil)
			store, err := client.Repository(t.Context(), oci.Repository{Registry: fixture.host, Path: "acme/plugin"})
			require.NoError(t, err)
			subject, removed := pushAttachment(t, store)
			// With no other attachment, the old index stays tagged and still lists the attachment.
			listed := []string{removed.Digest.String()}
			if extra {
				kept, keptRaw := attachmentManifest(t, &subject, "kept")
				require.NoError(t, store.Push(t.Context(), kept, strings.NewReader(string(keptRaw))))
				// The replacement index lists only the kept attachment, but the removed one was never deleted.
				listed = []string{kept.Digest.String()}
			}
			fixture.mu.Lock()
			fixture.allowDelete[removed.Digest.String()] = true
			fixture.mu.Unlock()
			ref, err := name.NewDigest(fixture.host + "/acme/plugin@" + removed.Digest.String())
			require.NoError(t, err)

			err = client.Delete(t.Context(), ref)

			require.Error(t, err, "the delete must not report success")
			assert.ErrorIs(t, err, errUtils.ErrOCIRegistry)
			fresh, err := client.Repository(t.Context(), oci.Repository{Registry: fixture.host, Path: "acme/plugin"})
			require.NoError(t, err)
			exists, err := fresh.Exists(t.Context(), removed)
			require.NoError(t, err)
			assert.True(t, exists, "the target DELETE was never sent, so the attachment still exists")
			referrers, err := fresh.Referrers(t.Context(), subject, "")
			require.NoError(t, err)
			assert.Equal(t, listed, referrerDigests(referrers))
		})
	}
}
