package sign

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/oci/signature"
	"github.com/cloudposse/atmos/pkg/oci/signature/signaturetest"
)

// Re-signing an unchanged release on GHCR replaces the tag-schema referrers index, and GHCR refuses to
// delete the replaced index with 405. The new signature is attached and listed before that DELETE, so the
// re-sign succeeds and both signatures are discoverable (#3760). Before the fix every re-sign failed, which
// stopped `atmos plugin release` before it reached the next package in the inventory.
func TestReSignSucceedsWhenTheRegistryRefusesToDeleteTheReplacedReferrersIndex(t *testing.T) {
	var refusedDeletes atomic.Int32
	backing := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/referrers/"):
			// GHCR does not serve the OCI 1.1 referrers API, so attachments use the tag schema.
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/manifests/"):
			refusedDeletes.Add(1)
			w.WriteHeader(http.StatusMethodNotAllowed)
		default:
			backing.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := oci.NewClient(nil)
	repo, err := name.NewRepository(strings.TrimPrefix(server.URL, "http://") + "/test/plugin")
	require.NoError(t, err)
	index := fixtureIndex(t)
	require.NoError(t, client.WriteIndex(t.Context(), repo.Tag("1.0.0"), index))
	digest, err := index.Digest()
	require.NoError(t, err)
	raw, err := index.RawManifest()
	require.NoError(t, err)
	authority := signaturetest.New(t)
	first := authority.Sign(t, "builder@example.com", "https://issuer.example.com", raw)
	second := authority.Sign(t, "builder@example.com", "https://issuer.example.com", raw)
	signer := NewMockBlobSigner(gomock.NewController(t))
	gomock.InOrder(
		signer.EXPECT().Sign(gomock.Any(), raw).Return(first, nil),
		signer.EXPECT().Sign(gomock.Any(), raw).Return(second, nil),
	)
	service := Service{Registry: client, Signer: signer}
	reference := repo.Digest(digest.String())

	_, firstErr := service.Release(t.Context(), reference.Name())
	require.NoError(t, firstErr, "the first signature creates the referrers index, so nothing is deleted")
	require.Zero(t, refusedDeletes.Load(), "precondition: the first signature does not replace an index")
	release, err := service.Release(t.Context(), reference.Name())

	require.NoError(t, err, "re-signing must not fail because the replaced index could not be deleted")
	assert.Equal(t, digest.String(), release.Digest)
	assert.Positive(t, refusedDeletes.Load(), "the re-sign asked the registry to delete the replaced index")
	evidence, err := signature.Fetch(t.Context(), client, reference)
	require.NoError(t, err)
	assert.ElementsMatch(t, [][]byte{first, second}, evidence, "both signatures are attached to the release")
}
