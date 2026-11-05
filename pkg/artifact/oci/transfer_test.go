//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"context"
	"errors"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/oci/ocitest"
)

var multiPlatform = []v1.Platform{{OS: "linux", Architecture: "amd64"}, {OS: "linux", Architecture: "arm64"}, {OS: "darwin", Architecture: "arm64"}}

// attachBundle stores a placeholder Sigstore bundle as a referrer of the pinned root, so a
// transfer has signature material to carry.
func attachBundle(t *testing.T, repo *Repository, pinned artifact.PinnedReference, bundle string) ocispec.Descriptor {
	t.Helper()
	st, loaded, err := repo.rootOf(context.Background(), &pinned)
	require.NoError(t, err)
	subject := artifact.SignatureSubject{Kind: artifact.SubjectOCIRoot, Digest: loaded.desc.Digest, MediaType: loaded.desc.MediaType, Size: loaded.desc.Size}
	payload := &artifact.EvidencePayload{Format: artifact.EvidenceSigstoreBundle, MediaType: bundleMediaTypePrefix + "v0.3+json", Data: []byte(bundle)}
	evidence, _, err := repo.storeAttachment(context.Background(), st, &attachRequest{root: loaded.desc, subject: subject, payload: payload})
	require.NoError(t, err)
	return evidence.Descriptor
}

// A native multi-platform image moves registry -> layout -> second registry with distinct
// credentials and the source shut down in between. Every original digest and referrer survives.
func TestNativeImageSurvivesRegistryLayoutRegistryWithDistinctCredentials(t *testing.T) {
	ctx := context.Background()
	sourceRegistry := newRegistry(t, &ocitest.Credential{Username: "source-user", Password: "source-secret"})
	targetRegistry := newRegistry(t, &ocitest.Credential{Username: "target-user", Password: "target-secret"})
	source := newRegistryRepo(t, sourceRegistry)
	carrier := newLayoutRepo(t)
	target := newRegistryRepo(t, targetRegistry)

	image := pushIndex(t, sourceRegistry, testPrefix+"/tools/img", "v1", multiPlatform...)
	ref := artifact.Ref{Name: "tools/img", Version: "v1"}
	sourcePinned := mustResolveRef(t, source, ref)
	assert.Equal(t, image.manifest, sourcePinned.ResolvedDigest)
	attached := attachBundle(t, source, sourcePinned, `{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json"}`)
	rawRoot := rawRootBytes(t, source, sourcePinned)

	require.True(t, carrier.CanCopyFrom(source))
	hop, err := carrier.CopyFrom(ctx, source, sourcePinned, artifact.CopyFromOptions{})
	require.NoError(t, err)
	assert.Equal(t, image.manifest, hop.Pinned.ResolvedDigest, "the layout keeps the original index digest")
	assert.Equal(t, artifact.PackagingOCIGraph, hop.Representation.Packaging)

	sourceRegistry.server.Close()

	carrierPinned := mustResolveRef(t, carrier, ref)
	hop, err = target.CopyFrom(ctx, carrier, carrierPinned, artifact.CopyFromOptions{})
	require.NoError(t, err)
	assert.Equal(t, image.manifest, hop.Pinned.ResolvedDigest, "the second registry resolves to the same root digest")

	targetPinned := mustResolveRef(t, target, ref)
	assert.Equal(t, rawRoot, rawRootBytes(t, target, targetPinned), "the original index bytes are untouched")
	content, err := target.Read(ctx, targetPinned)
	require.NoError(t, err)
	defer content.Close()
	nodes := walkGraph(t, content.Graph)
	assert.GreaterOrEqual(t, nodes, 1+len(multiPlatform)*(1+1+2), "index, each platform manifest, config and two layers all arrived")

	inTarget, err := target.attachments(ctx, mustStore(t, target, ref.Name), ref.Name, targetPinnedDescriptor(t, target, targetPinned))
	require.NoError(t, err)
	require.Len(t, inTarget.valid, 1, "the signature referrer travelled with the image")
	assert.Equal(t, attached.Digest, inTarget.valid[0].layer.Digest)
	assert.Empty(t, inTarget.diagnostics)
}

func rawRootBytes(t *testing.T, repo *Repository, pinned artifact.PinnedReference) []byte {
	t.Helper()
	_, loaded, err := repo.rootOf(context.Background(), &pinned)
	require.NoError(t, err)
	return loaded.raw
}

func mustStore(t *testing.T, repo *Repository, name string) store {
	t.Helper()
	st, err := repo.be.open(context.Background(), name, false)
	require.NoError(t, err)
	return st
}

func targetPinnedDescriptor(t *testing.T, repo *Repository, pinned artifact.PinnedReference) ocispec.Descriptor {
	t.Helper()
	_, loaded, err := repo.rootOf(context.Background(), &pinned)
	require.NoError(t, err)
	return loaded.desc
}

// The committed plugin release moves registry -> layout -> registry with its digests intact.
func TestPluginReleaseTransfersWithDigestsPreserved(t *testing.T) {
	ctx := context.Background()
	sourceRegistry := newRegistry(t, &ocitest.Credential{Username: "a", Password: "pa"})
	targetRegistry := newRegistry(t, &ocitest.Credential{Username: "b", Password: "pb"})
	source, carrier, target := newRegistryRepo(t, sourceRegistry), newLayoutRepo(t), newRegistryRepo(t, targetRegistry)

	ref := artifact.Ref{Name: "plugins/example", Version: "1.2.3"}
	importPlugin(t, source, ref)
	_, err := carrier.CopyFrom(ctx, source, mustResolveRef(t, source, ref), artifact.CopyFromOptions{})
	require.NoError(t, err)
	sourceRegistry.server.Close()
	result, err := target.CopyFrom(ctx, carrier, mustResolveRef(t, carrier, ref), artifact.CopyFromOptions{})
	require.NoError(t, err)
	assert.Equal(t, pluginRoot, result.Pinned.ResolvedDigest)
	pinned := mustResolveRef(t, target, ref)
	assert.Equal(t, pluginRoot, pinned.ResolvedDigest)
	inspection, err := target.Inspect(ctx, pinned)
	require.NoError(t, err)
	assert.Equal(t, "example/fixture", inspection.Representation.OriginalRoot.Annotations[pluginAnnotationName])
}

// An Atmos package copied between OCI repositories is not repackaged: the envelope digest is
// identical too. Repacking through the service (tested elsewhere) keeps only Record identity.
func TestAtmosPackageNativeCopyKeepsTheEnvelopeDigest(t *testing.T) {
	ctx := context.Background()
	source, carrier := newRegistryRepo(t, newRegistry(t, nil)), newLayoutRepo(t)
	published := publish(t, source, publishSpec{content: "native copy"})

	result, err := carrier.CopyFrom(ctx, source, mustResolveRef(t, source, artifact.Ref{Name: appName, Version: appVersion}), artifact.CopyFromOptions{})
	require.NoError(t, err)
	assert.Equal(t, published.Pinned.ResolvedDigest, result.Pinned.ResolvedDigest)
	assert.Equal(t, published.Pinned.RecordDigest, result.Pinned.RecordDigest)
	assert.Equal(t, published.Pinned.ContentDigest, result.Pinned.ContentDigest)

	// By digest, a copy publishes under the Record's own name and version.
	byDigest := artifact.PinnedReference{Ref: artifact.Ref{Name: appName, Digest: published.Pinned.RecordDigest}, ResolvedDigest: published.Pinned.ResolvedDigest}
	other := newLayoutRepo(t)
	copied, err := other.CopyFrom(ctx, source, byDigest, artifact.CopyFromOptions{})
	require.NoError(t, err)
	assert.Equal(t, appVersion, copied.Pinned.Ref.Version)
	assert.False(t, other.CanCopyFrom(other))
	assert.False(t, other.CanCopyFrom(nil))
}

func TestCopyRefusesToDropReferrersItCannotList(t *testing.T) {
	ctx := context.Background()
	reg := newRegistry(t, nil)
	healthy := newRegistryRepo(t, reg)
	image := pushImage(t, reg, testPrefix+"/tools/img", "v1")
	ref := artifact.Ref{Name: "tools/img", Version: "v1"}
	pinned := mustResolveRef(t, healthy, ref)
	attachBundle(t, healthy, pinned, `{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json"}`)

	broken, err := NewRepository(ctx, "broken", registryConfig(reg.server.Host), WithTransport(noReferrers{reg.transport}), WithLockDir(t.TempDir()))
	require.NoError(t, err)
	destination := newLayoutRepo(t)
	_, err = destination.CopyFrom(ctx, broken, mustResolveRef(t, broken, ref), artifact.CopyFromOptions{})
	require.Error(t, err, "a registry that cannot provide the signature material fails the copy")
	assert.True(t, errors.Is(err, errUtils.ErrArtifactUploadFailed))
	_, err = destination.Resolve(ctx, ref)
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "nothing was published, not even the unsigned image")
	assert.NotEmpty(t, image.manifest)
}

// noReferrers is a transport whose repositories fail every referrer lookup.
type noReferrers struct{ atmosoci.Transport }

func (n noReferrers) Repository(ctx context.Context, repository atmosoci.Repository) (atmosoci.RepositoryStore, error) {
	inner, err := n.Transport.Repository(ctx, repository)
	if err != nil {
		return nil, err
	}
	return brokenReferrers{inner}, nil
}

type brokenReferrers struct{ atmosoci.RepositoryStore }

func (brokenReferrers) Predecessors(context.Context, ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	return nil, errors.New("referrers are unavailable")
}

func (brokenReferrers) Referrers(context.Context, ocispec.Descriptor, string) ([]ocispec.Descriptor, error) {
	return nil, errors.New("referrers are unavailable")
}

// A registry without the OCI 1.1 referrers API uses the tag-schema fallback, and signature
// material still travels with the image.
func TestReferrersTagSchemaFallbackStillCarriesSignatures(t *testing.T) {
	ctx := context.Background()
	server := ocitest.NewServerWithOptions(t, nil, ocitest.WithoutReferrersAPI())
	client := atmosoci.NewClient(nil, atmosoci.WithKeychain(credentialsKeychain{}))
	reg := &registry{server: server, recorder: &recorder{}, transport: client}
	source := newRegistryRepo(t, reg)
	pushImage(t, reg, testPrefix+"/tools/img", "v1")
	ref := artifact.Ref{Name: "tools/img", Version: "v1"}
	attached := attachBundle(t, source, mustResolveRef(t, source, ref), `{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json","fallback":true}`)

	carrier := newLayoutRepo(t)
	_, err := carrier.CopyFrom(ctx, source, mustResolveRef(t, source, ref), artifact.CopyFromOptions{})
	require.NoError(t, err)
	pinned := mustResolveRef(t, carrier, ref)
	set, err := carrier.attachments(ctx, mustStore(t, carrier, ref.Name), ref.Name, targetPinnedDescriptor(t, carrier, pinned))
	require.NoError(t, err)
	require.Len(t, set.valid, 1)
	assert.Equal(t, attached.Digest, set.valid[0].layer.Digest)
}

func TestSelectingAPlatformKeepsTheOriginalIndex(t *testing.T) {
	ctx := context.Background()
	reg := newRegistry(t, nil)
	repo := newRegistryRepo(t, reg)
	image := pushIndex(t, reg, testPrefix+"/tools/img", "v1", multiPlatform...)
	pinned := mustResolveRef(t, repo, artifact.Ref{Name: "tools/img", Version: "v1"})

	selected, child, err := repo.SelectPlatform(ctx, pinned, ocispec.Platform{OS: "linux", Architecture: "arm64"})
	require.NoError(t, err)
	assert.Equal(t, image.manifest, selected.ResolvedDigest, "selection never reconstructs the root")
	assert.Equal(t, image.children[1], child.Digest)
	require.NotNil(t, selected.Platform)

	content, err := repo.Read(ctx, selected)
	require.NoError(t, err)
	defer content.Close()
	assert.Equal(t, image.manifest, content.Graph.Root().Digest)
	successors, err := content.Graph.Successors(ctx, content.Graph.Root())
	require.NoError(t, err)
	require.Len(t, successors, 1, "only the selected child travels below the original index")
	assert.Equal(t, image.children[1], successors[0].Digest)
	assert.Equal(t, 1+4, walkGraph(t, content.Graph), "index, one manifest, its config and two layers")

	inspection, err := repo.Inspect(ctx, selected)
	require.NoError(t, err)
	assert.Equal(t, image.manifest, inspection.Representation.OriginalRoot.Digest)
	assert.Len(t, inspection.Representation.Payload, 1)

	_, _, err = repo.SelectPlatform(ctx, pinned, ocispec.Platform{OS: "plan9", Architecture: "mips"})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, err = newLayoutRepo(t).CopyFrom(ctx, repo, selected, artifact.CopyFromOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "a native copy does not narrow an index")
}

func TestCrossBackendCredentialsAreNotShared(t *testing.T) {
	ctx := context.Background()
	protected := newRegistry(t, &ocitest.Credential{Username: "u", Password: "p"})
	pushImage(t, protected, testPrefix+"/tools/img", "v1")

	anonymous := atmosoci.NewClient(nil, atmosoci.WithKeychain(credentialsKeychain{}))
	repo, err := NewRepository(ctx, "wrong", registryConfig(protected.server.Host), WithTransport(anonymous), WithLockDir(t.TempDir()))
	require.NoError(t, err)
	_, err = repo.Resolve(ctx, artifact.Ref{Name: "tools/img", Version: "v1"})
	require.Error(t, err)
	assert.NotErrorIs(t, err, errUtils.ErrArtifactNotFound, "an authentication failure is not reported as a missing artifact")
	assert.Contains(t, err.Error(), "credential")
}
