//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// alias tags an existing manifest with another version, creating a sibling reference.
func alias(t *testing.T, repo *Repository, pinned artifact.PinnedReference, version string) {
	t.Helper()
	st, err := repo.be.open(context.Background(), pinned.Ref.Name, true)
	require.NoError(t, err)
	desc, ok := repo.descriptorFor(pinned.Ref.Name, pinned.ResolvedDigest)
	require.True(t, ok)
	require.NoError(t, st.Tag(context.Background(), desc, repo.be.reference(pinned.Ref.Name, version)))
}

func resolves(repo *Repository, name, version string) bool {
	_, err := repo.Resolve(context.Background(), artifact.Ref{Name: name, Version: version})
	return err == nil
}

func TestRegistryTagDeleteRefusesWhenItWouldRemoveASiblingTag(t *testing.T) {
	repo := newRegistryRepo(t, newRegistry(t, nil))
	first := publish(t, repo, publishSpec{content: "shared manifest"})
	alias(t, repo, first.Pinned, "stable")
	pinned, err := repo.Resolve(context.Background(), artifact.Ref{Name: appName, Version: appVersion})
	require.NoError(t, err)

	_, err = repo.Delete(context.Background(), pinned, artifact.DeleteOptions{ExpectedCurrent: &pinned, Scope: artifact.DeleteManifest})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "only manifest deletion exists and it would remove stable")
	assert.Contains(t, err.Error(), "stable")
	assert.True(t, resolves(repo, appName, appVersion))
	assert.True(t, resolves(repo, appName, "stable"))
}

func TestRegistryDigestDeleteIsManifestScopedAndReportsAffectedReferences(t *testing.T) {
	repo := newRegistryRepo(t, newRegistry(t, nil))
	first := publish(t, repo, publishSpec{content: "shared manifest"})
	alias(t, repo, first.Pinned, "stable")
	publish(t, repo, publishSpec{version: "2.0.0", content: "unrelated"})

	byDigest := artifact.PinnedReference{Ref: artifact.Ref{Name: appName, Digest: first.Pinned.ResolvedDigest}, ResolvedDigest: first.Pinned.ResolvedDigest}
	result, err := repo.Delete(context.Background(), byDigest, artifact.DeleteOptions{Scope: artifact.DeleteManifest})
	require.NoError(t, err)
	assert.Equal(t, first.Pinned.ResolvedDigest, result.Digest)
	assert.ElementsMatch(t, []artifact.Ref{{Name: appName, Version: appVersion}, {Name: appName, Version: "stable"}}, result.Affected)
	assert.False(t, resolves(repo, appName, appVersion))
	assert.False(t, resolves(repo, appName, "stable"))
	assert.True(t, resolves(repo, appName, "2.0.0"), "deletion is scoped to the selected manifest")
}

func TestRegistryTagDeleteWithoutSiblingsKeepsBlobsAndOtherVersions(t *testing.T) {
	reg := newRegistry(t, nil)
	repo := newRegistryRepo(t, reg)
	drop := publish(t, repo, publishSpec{content: "drop"})
	publish(t, repo, publishSpec{version: "2.0.0", content: "keep"})
	pinned, err := repo.Resolve(context.Background(), artifact.Ref{Name: appName, Version: appVersion})
	require.NoError(t, err)

	reg.recorder.reset()
	result, err := repo.Delete(context.Background(), pinned, artifact.DeleteOptions{ExpectedCurrent: &pinned, Scope: artifact.DeleteManifest})
	require.NoError(t, err)
	assert.Empty(t, result.Affected)
	assert.NotEmpty(t, result.Deleted)
	assert.Zero(t, reg.recorder.count("DELETE", "/blobs/"), "shared blobs are never deleted directly")
	assert.False(t, resolves(repo, appName, appVersion))
	assert.True(t, resolves(repo, appName, "2.0.0"))
	assert.NotEqual(t, drop.Pinned.ResolvedDigest, mustResolve(t, repo, "2.0.0").ResolvedDigest)
}

func mustResolve(t *testing.T, repo *Repository, version string) artifact.PinnedReference {
	t.Helper()
	pinned, err := repo.Resolve(context.Background(), artifact.Ref{Name: appName, Version: version})
	require.NoError(t, err)
	return pinned
}

func TestLayoutTagDeleteRemovesOnlyTheNamedReference(t *testing.T) {
	repo := newLayoutRepo(t)
	first := publish(t, repo, publishSpec{content: "shared"})
	alias(t, repo, first.Pinned, "stable")
	pinned := mustResolve(t, repo, appVersion)

	result, err := repo.Delete(context.Background(), pinned, artifact.DeleteOptions{ExpectedCurrent: &pinned, Scope: artifact.DeleteReference})
	require.NoError(t, err)
	assert.Empty(t, result.Affected)
	assert.False(t, resolves(repo, appName, appVersion))
	assert.True(t, resolves(repo, appName, "stable"), "the sibling reference and the shared blobs survive")
	content, err := repo.Read(context.Background(), pinned)
	require.NoError(t, err, "the pinned manifest and its blobs are still readable")
	require.NoError(t, content.Close())

	byDigest := artifact.PinnedReference{Ref: artifact.Ref{Name: appName, Digest: first.Pinned.ResolvedDigest}, ResolvedDigest: first.Pinned.ResolvedDigest}
	result, err = repo.Delete(context.Background(), byDigest, artifact.DeleteOptions{Scope: artifact.DeleteReference})
	require.NoError(t, err)
	assert.Equal(t, []artifact.Ref{{Name: appName, Version: "stable"}}, result.Affected)
	assert.False(t, resolves(repo, appName, "stable"))
}

func TestDeleteRejectsAStaleExpectedCurrent(t *testing.T) {
	backends(t, func(t *testing.T, repo *Repository) {
		publish(t, repo, publishSpec{content: "current"})
		pinned := mustResolve(t, repo, appVersion)
		stale := pinned
		stale.ResolvedDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
		_, err := repo.Delete(context.Background(), pinned, artifact.DeleteOptions{ExpectedCurrent: &stale, Scope: repo.Capabilities().Delete})
		require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
		assert.True(t, resolves(repo, appName, appVersion))
	})
}

func TestDeleteRefusesAMovedTag(t *testing.T) {
	repo := newLayoutRepo(t)
	first := publish(t, repo, publishSpec{content: "one"})
	old := mustResolve(t, repo, appVersion)
	_, err := repo.Publish(context.Background(), prepare(t, publishSpec{content: "two"}), artifact.PublishOptions{ExpectedPrior: &first.Pinned, Force: true})
	require.NoError(t, err)
	_, err = repo.Delete(context.Background(), old, artifact.DeleteOptions{Scope: artifact.DeleteReference})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "the tag no longer names the pinned manifest")
}

func TestRegistryRejectsReferenceScope(t *testing.T) {
	repo := newRegistryRepo(t, newRegistry(t, nil))
	first := publish(t, repo, publishSpec{content: "keep"})
	_, err := repo.Delete(context.Background(), first.Pinned, artifact.DeleteOptions{Scope: artifact.DeleteReference})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	assert.True(t, resolves(repo, appName, appVersion))
}

// scanHookBackend runs a hook right after the backend lists the versions of a name, which is the
// moment a concurrent client can add a tag between the sibling scan and the removal.
type scanHookBackend struct {
	backend
	mu    sync.Mutex
	scans int
	after func(scan int)
}

func (b *scanHookBackend) versions(ctx context.Context, name string) ([]string, error) {
	versions, err := b.backend.versions(ctx, name)
	b.mu.Lock()
	b.scans++
	scan, hook := b.scans, b.after
	b.mu.Unlock()
	if hook != nil {
		hook(scan)
	}
	return versions, err
}

// A tag another client adds after the first sibling scan must still stop a manifest-scoped tag
// delete: the siblings are checked again immediately before the manifest is removed.
func TestRegistryTagDeleteRechecksSiblingsRightBeforeRemoving(t *testing.T) {
	repo := newRegistryRepo(t, newRegistry(t, nil))
	first := publish(t, repo, publishSpec{content: "shared manifest"})
	pinned := mustResolve(t, repo, appVersion)

	hooked := &scanHookBackend{backend: repo.be}
	repo.be = hooked
	hooked.after = func(scan int) {
		if scan == 1 {
			alias(t, repo, first.Pinned, "stable")
		}
	}

	_, err := repo.Delete(context.Background(), pinned, artifact.DeleteOptions{ExpectedCurrent: &pinned, Scope: artifact.DeleteManifest})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "a tag added after the first scan is found by the second")
	assert.Contains(t, err.Error(), "stable")
	assert.True(t, resolves(repo, appName, appVersion), "nothing was removed")
	assert.True(t, resolves(repo, appName, "stable"), "the tag added concurrently survives")
}

// A digest delete reports the references that exist when the manifest is removed, not the ones the
// first scan happened to see.
func TestRegistryDigestDeleteReportsTagsAddedAfterTheFirstScan(t *testing.T) {
	repo := newRegistryRepo(t, newRegistry(t, nil))
	first := publish(t, repo, publishSpec{content: "shared manifest"})
	hooked := &scanHookBackend{backend: repo.be}
	repo.be = hooked
	hooked.after = func(scan int) {
		if scan == 1 {
			alias(t, repo, first.Pinned, "stable")
		}
	}

	byDigest := artifact.PinnedReference{Ref: artifact.Ref{Name: appName, Digest: first.Pinned.ResolvedDigest}, ResolvedDigest: first.Pinned.ResolvedDigest}
	result, err := repo.Delete(context.Background(), byDigest, artifact.DeleteOptions{Scope: artifact.DeleteManifest})
	require.NoError(t, err)
	assert.ElementsMatch(t, []artifact.Ref{{Name: appName, Version: appVersion}, {Name: appName, Version: "stable"}}, result.Affected, "the tag added after the first scan is reported")
	assert.False(t, resolves(repo, appName, "stable"))
}
