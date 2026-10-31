//nolint:gocritic // Descriptor-by-value parameters mirror the ORAS content contracts.
package oci_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/oci/ocitest"
	"github.com/cloudposse/atmos/pkg/oci/signature"
	"github.com/cloudposse/atmos/pkg/oci/signature/signaturetest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

// pluginReleaseIndexDigest is the pinned root digest of the committed A00 plugin release fixture
// (pkg/artifact/compat/testdata/plugin-release). It must never change.
const pluginReleaseIndexDigest = "sha256:af34a37555cc97c7f1043fdb2ad3404a062d97d1ce07d96c5a594f6b5ac83b59"

func pluginFixtureDirectory() string {
	return filepath.Join("..", "artifact", "compat", "testdata", "plugin-release")
}

// authenticatedRegistry starts a registry that accepts exactly one identity and returns a client
// that can only present that identity.
func authenticatedRegistry(t *testing.T, password string) (*ocitest.Server, *oci.Client) {
	t.Helper()
	server := ocitest.NewServer(t, &ocitest.Credential{Username: "publisher", Password: password})
	store, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	chain := oci.CredentialKeychain{Store: store}
	require.NoError(t, chain.Login(server.Host, &authn.AuthConfig{Username: "publisher", Password: password}))
	return server, oci.NewClient(nil, oci.WithKeychain(chain))
}

func openRepository(t *testing.T, client *oci.Client, host string) oci.RepositoryStore {
	t.Helper()
	store, err := client.Repository(t.Context(), oci.Repository{Registry: host, Path: "acme/plugin"})
	require.NoError(t, err)
	return store
}

// readTree returns every blob below a layout, keyed by file name.
func readBlobs(t *testing.T, directory string) map[string][]byte {
	t.Helper()
	blobs := map[string][]byte{}
	root := filepath.Join(directory, "blobs", "sha256")
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		blobs[entry.Name()] = content
		return err
	}))
	require.NotEmpty(t, blobs)
	return blobs
}

// attachBundle publishes a Sigstore bundle as a subject-bound referrer, exactly like plugin signing.
func attachBundle(t *testing.T, client *oci.Client, repository name.Repository, subject *v1.Descriptor, raw []byte) {
	t.Helper()
	image, err := mutate.AppendLayers(empty.Image, static.NewLayer(raw, types.MediaType("application/json")))
	require.NoError(t, err)
	image = mutate.MediaType(image, types.OCIManifestSchema1)
	image = mutate.ConfigMediaType(image, types.MediaType("application/vnd.dev.sigstore.bundle.v0.3+json"))
	image = mutate.Subject(image, *subject).(v1.Image)
	digest, err := image.Digest()
	require.NoError(t, err)
	require.NoError(t, client.WriteImage(t.Context(), repository.Digest(digest.String()), image))
}

type roundTripCase struct {
	name       string
	prepare    func(t *testing.T) (directory, digest string)
	inspectors func(t *testing.T, client *oci.Client, repository name.Repository, digest string) any
}

func twoPlatformLayout(t *testing.T) (string, string) {
	t.Helper()
	index := testIndex(t)
	directory := filepath.Join(t.TempDir(), "source")
	require.NoError(t, oci.SaveIndex(directory, index))
	digest, err := index.Digest()
	require.NoError(t, err)
	return directory, digest.String()
}

// inspectedRelease captures what plugin verification derives from a release.
func inspectedRelease(t *testing.T, client *oci.Client, repository name.Repository, digest string) any {
	t.Helper()
	descriptor, err := client.Get(t.Context(), repository.Digest(digest))
	require.NoError(t, err)
	index, err := descriptor.ImageIndex()
	require.NoError(t, err)
	release, err := pluginoci.Inspect(index, digest)
	require.NoError(t, err)
	platforms := map[string]string{}
	for platform, image := range release.Platforms {
		platforms[platform] = image.Digest.String()
	}
	return map[string]any{"name": release.Name, "version": release.Version, "digest": release.Digest, "platforms": platforms}
}

func manifestDigests(t *testing.T, client *oci.Client, repository name.Repository, digest string) any {
	t.Helper()
	descriptor, err := client.Get(t.Context(), repository.Digest(digest))
	require.NoError(t, err)
	index, err := descriptor.ImageIndex()
	require.NoError(t, err)
	manifest, err := index.IndexManifest()
	require.NoError(t, err)
	var digests []string
	for i := range manifest.Manifests {
		digests = append(digests, manifest.Manifests[i].Digest.String()+"/"+manifest.Manifests[i].Platform.Architecture)
	}
	return digests
}

// TestRegistryLayoutRegistryRoundTripKeepsDigestsAndVerification moves releases registry A ->
// OCI layout -> registry B. The registries accept different credentials and A is shut down
// before the final hop, so the layout alone must carry the complete graph, including the signature
// referrer. Digests, blobs, release inspection, and signature verification must not change.
func TestRegistryLayoutRegistryRoundTripKeepsDigestsAndVerification(t *testing.T) {
	ctx := t.Context()
	for _, tc := range []roundTripCase{
		{"plugin release fixture", func(t *testing.T) (string, string) { return pluginFixtureDirectory(), pluginReleaseIndexDigest }, inspectedRelease},
		{"two-platform index", twoPlatformLayout, manifestDigests},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sourceDirectory, digest := tc.prepare(t)
			sourceBlobs := readBlobs(t, sourceDirectory)
			source, err := oci.OpenLayoutReadOnly(ctx, sourceDirectory)
			require.NoError(t, err)
			rootDescriptor, err := source.Resolve(ctx, digest)
			require.NoError(t, err)
			require.Equal(t, digest, rootDescriptor.Digest.String())

			// Registry A: publish the release and a Sigstore bundle bound to its digest.
			serverA, clientA := authenticatedRegistry(t, "secret-a")
			storeA := openRepository(t, clientA, serverA.Host)
			_, err = oci.Copy(ctx, oci.Source{Store: source, Reference: digest}, oci.Destination{Store: storeA, Reference: "1.2.3"})
			require.NoError(t, err)
			repositoryA, err := name.NewRepository(serverA.Host + "/acme/plugin")
			require.NoError(t, err)
			head, err := clientA.Head(ctx, repositoryA.Tag("1.2.3"))
			require.NoError(t, err)
			require.Equal(t, digest, head.Digest.String(), "pushing must not alter the release digest")

			rawRoot, err := oci.FetchBytes(ctx, source, rootDescriptor, 1<<20)
			require.NoError(t, err)
			authority := signaturetest.New(t)
			identity := signature.Identity{Identity: "https://github.com/acme/plugin/.github/workflows/release.yml@refs/tags/v1.2.3", Issuer: "https://token.actions.githubusercontent.com"}
			bundle := authority.Sign(t, identity.Identity, identity.Issuer, rawRoot)
			verifier := signature.Verifier{LoadRoot: func(context.Context, bool) (root.TrustedMaterial, error) { return authority, nil }}
			want, err := verifier.Verify(ctx, bundle, digest, []signature.Identity{identity}, true)
			require.NoError(t, err)
			attachBundle(t, clientA, repositoryA, head, bundle)
			before := tc.inspectors(t, clientA, repositoryA, digest)

			// Hop 1: registry A -> layout, with referrers.
			layoutDirectory := filepath.Join(t.TempDir(), "layout")
			layout, err := oci.OpenLayout(ctx, layoutDirectory)
			require.NoError(t, err)
			copied, err := oci.ExtendedCopy(ctx, oci.GraphSource{Store: storeA, Reference: "1.2.3"}, oci.Destination{Store: layout, Reference: "1.2.3"})
			require.NoError(t, err)
			require.Equal(t, digest, copied.Digest.String())
			require.Equal(t, []string{"1.2.3"}, layout.References())
			layoutBlobs := readBlobs(t, layoutDirectory)
			for name, content := range sourceBlobs {
				require.Equal(t, content, layoutBlobs[name], "blob %s changed in transit", name)
			}
			require.Len(t, layoutBlobs, len(sourceBlobs)+3, "signature manifest, config, and bundle layer travel with the release")

			// Registry A disappears; registry B has different credentials.
			serverA.Close()
			serverB, clientB := authenticatedRegistry(t, "secret-b")
			storeB := openRepository(t, clientB, serverB.Host)

			// Hop 2 reads the layout from disk again, proving the on-disk state is self-sufficient.
			reopened, err := oci.OpenLayout(ctx, layoutDirectory)
			require.NoError(t, err)
			_, err = oci.ExtendedCopy(ctx, oci.GraphSource{Store: reopened, Reference: "1.2.3"}, oci.Destination{Store: storeB, Reference: "1.2.3"})
			require.NoError(t, err)

			repositoryB, err := name.NewRepository(serverB.Host + "/acme/plugin")
			require.NoError(t, err)
			after, err := clientB.Head(ctx, repositoryB.Tag("1.2.3"))
			require.NoError(t, err)
			require.Equal(t, digest, after.Digest.String())
			require.Equal(t, before, tc.inspectors(t, clientB, repositoryB, digest))
			tags, err := clientB.Tags(ctx, repositoryB)
			require.NoError(t, err)
			// The registry fixture lacks the OCI-Subject header, so the attachment is also indexed
			// under the standard referrers tag-schema fallback tag.
			require.ElementsMatch(t, []string{"1.2.3", "sha256-" + strings.TrimPrefix(digest, "sha256:")}, tags)

			bundles, err := signature.Fetch(ctx, clientB, repositoryB.Digest(digest))
			require.NoError(t, err)
			require.Equal(t, [][]byte{bundle}, bundles)
			got, err := verifier.Verify(ctx, bundles[0], digest, []signature.Identity{identity}, true)
			require.NoError(t, err)
			require.Equal(t, want, got, "verification result must survive the move")

			// Registry B's credentials never worked on A, and A's never work on B.
			_, err = oci.Copy(ctx, oci.Source{Store: reopened, Reference: "1.2.3"}, oci.Destination{Store: openRepository(t, clientA, serverB.Host), Reference: "again"})
			require.ErrorIs(t, err, errUtils.ErrOCIRegistry)
		})
	}
}

func TestLayoutHoldsSeveralReferencesWithoutWeakeningLoadIndex(t *testing.T) {
	ctx := t.Context()
	twoPlatformDirectory, twoPlatformDigest := twoPlatformLayout(t)
	releaseStore, err := oci.OpenLayoutReadOnly(ctx, pluginFixtureDirectory())
	require.NoError(t, err)
	otherStore, err := oci.OpenLayoutReadOnly(ctx, twoPlatformDirectory)
	require.NoError(t, err)

	directory := filepath.Join(t.TempDir(), "layout")
	layout, err := oci.OpenLayout(ctx, directory)
	require.NoError(t, err)
	_, err = oci.Copy(ctx, oci.Source{Store: releaseStore, Reference: pluginReleaseIndexDigest}, oci.Destination{Store: layout, Reference: "plugin-1.2.3"})
	require.NoError(t, err)

	// One release in a layout still satisfies the single-release validation.
	index, err := oci.LoadIndex(directory, pluginReleaseIndexDigest)
	require.NoError(t, err)
	digest, err := index.Digest()
	require.NoError(t, err)
	assert.Equal(t, pluginReleaseIndexDigest, digest.String())

	_, err = oci.Copy(ctx, oci.Source{Store: otherStore, Reference: twoPlatformDigest}, oci.Destination{Store: layout, Reference: "two-platform"})
	require.NoError(t, err)
	assert.Equal(t, []string{"plugin-1.2.3", "two-platform"}, layout.References())
	_, err = oci.LoadIndex(directory, "")
	require.ErrorIs(t, err, errUtils.ErrOCILayout, "several references must still be rejected by LoadIndex")

	reopened, err := oci.OpenLayout(ctx, directory)
	require.NoError(t, err)
	assert.Equal(t, []string{"plugin-1.2.3", "two-platform"}, reopened.References())
	for reference, want := range map[string]string{"plugin-1.2.3": pluginReleaseIndexDigest, "two-platform": twoPlatformDigest} {
		resolved, err := reopened.Resolve(ctx, reference)
		require.NoError(t, err)
		assert.Equal(t, want, resolved.Digest.String(), reference)
	}

	// Re-tagging an existing reference replaces it rather than adding a duplicate entry.
	resolved, err := reopened.Resolve(ctx, "two-platform")
	require.NoError(t, err)
	require.NoError(t, reopened.Tag(ctx, resolved, "two-platform"))
	assert.Equal(t, []string{"plugin-1.2.3", "two-platform"}, reopened.References())
}

// TestLegacyWriteIndexPublishesTheExistingReleaseBytes proves the bridge serializes a legacy
// index's raw bytes: publishing the committed plugin release through the old Registry API must yield
// byte-identical blobs and the pinned digest, never a rebuilt release.
func TestLegacyWriteIndexPublishesTheExistingReleaseBytes(t *testing.T) {
	ctx := t.Context()
	index, err := oci.LoadIndex(pluginFixtureDirectory(), pluginReleaseIndexDigest)
	require.NoError(t, err)
	server, client := authenticatedRegistry(t, "secret")
	ref, err := name.NewTag(server.Host + "/acme/plugin:1.2.3")
	require.NoError(t, err)
	require.NoError(t, client.WriteIndex(ctx, ref, index))

	head, err := client.Head(ctx, ref)
	require.NoError(t, err)
	require.Equal(t, pluginReleaseIndexDigest, head.Digest.String())

	// Pull the registry's content back into a layout: every blob must equal the committed fixture.
	directory := filepath.Join(t.TempDir(), "pulled")
	layout, err := oci.OpenLayout(ctx, directory)
	require.NoError(t, err)
	_, err = oci.Copy(ctx, oci.Source{Store: openRepository(t, client, server.Host), Reference: "1.2.3"}, oci.Destination{Store: layout, Reference: "1.2.3"})
	require.NoError(t, err)
	want := readBlobs(t, pluginFixtureDirectory())
	got := readBlobs(t, directory)
	require.Len(t, got, len(want))
	for name, content := range want {
		require.Equal(t, content, got[name], "blob %s differs from the committed release", name)
	}

	descriptor, err := client.Get(ctx, ref)
	require.NoError(t, err)
	assert.Equal(t, pluginReleaseIndexDigest, descriptor.Digest.String())
	assert.NotNil(t, inspectedRelease(t, client, mustRepository(t, server.Host), pluginReleaseIndexDigest))
}

func mustRepository(t *testing.T, host string) name.Repository {
	t.Helper()
	repository, err := name.NewRepository(host + "/acme/plugin")
	require.NoError(t, err)
	return repository
}
