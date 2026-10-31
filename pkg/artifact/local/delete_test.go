package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

func TestDelete_IsScopedToTheSelectedArtifact(t *testing.T) {
	for _, spec := range allSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			repo := newRepo(t, spec)
			publish(t, repo, &prepareSpec{name: "team/keep", version: "1.0.0"}, artifact.PublishOptions{})
			publish(t, repo, &prepareSpec{name: "team/keep", version: "2.0.0", files: map[string]string{"data.txt": "two"}}, artifact.PublishOptions{})
			target := publish(t, repo, &prepareSpec{name: "team/drop", version: "1.0.0", files: map[string]string{"data.txt": "drop"}}, artifact.PublishOptions{})

			userFile := filepath.Join(repo.root, "notes", "user.txt")
			require.NoError(t, os.MkdirAll(filepath.Dir(userFile), 0o755))
			require.NoError(t, os.WriteFile(userFile, []byte("mine"), 0o644))
			outside := filepath.Join(filepath.Dir(repo.root), "outside.txt")
			require.NoError(t, os.WriteFile(outside, []byte("outside"), 0o644))

			pinned := resolve(t, repo, ref("team/drop", "1.0.0"))
			result, err := repo.Delete(context.Background(), pinned, artifact.DeleteOptions{ExpectedCurrent: &pinned, Scope: artifact.DeleteObject})
			require.NoError(t, err)
			assert.Equal(t, pinned.RecordDigest, result.Digest)
			require.NotEmpty(t, result.Deleted)
			assert.Equal(t, target.Location, result.Deleted[0])

			_, err = repo.Resolve(context.Background(), ref("team/drop", "1.0.0"))
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
			assertIntact(t, repo, ref("team/keep", "1.0.0"))
			assertIntact(t, repo, ref("team/keep", "2.0.0"))
			assert.FileExists(t, userFile, "files the repository does not manage are never touched")
			assert.FileExists(t, outside, "nothing outside the root is touched")
			assert.DirExists(t, repo.root)
			assert.NoDirExists(t, filepath.Join(repo.root, "team", "drop"), "empty parents of the deleted artifact are pruned")
			assert.DirExists(t, filepath.Join(repo.root, "team", "keep"))
			assert.NoFileExists(t, filepath.Join(repo.root, filepath.FromSlash(layout.RecordPath(pinned.RecordDigest))))
			assert.NoFileExists(t, filepath.Join(repo.root, filepath.FromSlash(layout.RepresentationPath(layout.ReferenceKey("team/drop", "1.0.0")))))
			assert.Equal(t, []string{"team/keep:1.0.0", "team/keep:2.0.0"}, listNames(t, repo))

			_, err = repo.Delete(context.Background(), pinned, artifact.DeleteOptions{Scope: artifact.DeleteObject})
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "deleting twice reports not found")
		})
	}
}

func TestDelete_LeavesUnmanagedFilesInsideADirectoryPayload(t *testing.T) {
	repo := newRepo(t, dirSpec())
	result, _ := publishFilePackage(t, repo)
	extra := filepath.Join(result.Location, "mine.txt")
	require.NoError(t, os.WriteFile(extra, []byte("user data"), 0o644))

	pinned := resolve(t, repo, defaultRef())
	_, err := repo.Delete(context.Background(), pinned, artifact.DeleteOptions{Scope: artifact.DeleteObject})
	require.NoError(t, err)
	assert.FileExists(t, extra, "delete removes what the artifact owns, not a user's extra file")
	assert.NoFileExists(t, filepath.Join(result.Location, "README.md"))
	assert.NoDirExists(t, filepath.Join(result.Location, ".atmos"))
	assert.Equal(t, []string{"team/app/1.0.0/mine.txt"}, payloadFiles(t, repo.root))
}

func TestDelete_RejectsStaleTargetsAndScopes(t *testing.T) {
	repo := newRepo(t, dirSpec())
	publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
	pinned := resolve(t, repo, defaultRef())

	stale := pinned
	stale.ResolvedDigest = digest.FromString("other")
	_, err := repo.Delete(context.Background(), pinned, artifact.DeleteOptions{ExpectedCurrent: &stale, Scope: artifact.DeleteObject})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	assertIntact(t, repo, defaultRef())

	for _, scope := range []artifact.DeleteSemantics{artifact.DeleteNone, artifact.DeleteManifest, artifact.DeleteReference} {
		_, err = repo.Delete(context.Background(), pinned, artifact.DeleteOptions{Scope: scope})
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, scope)
	}
	assertIntact(t, repo, defaultRef())
}

func TestDelete_DoesNotDeletePendingPublications(t *testing.T) {
	var fault armedFault
	spec := dirSpec()
	spec = spec.withOpts(fault.option())
	repo := newRepo(t, spec)
	publish(t, repo, &prepareSpec{files: map[string]string{"data.txt": "v1"}}, artifact.PublishOptions{})
	old := resolve(t, repo, defaultRef())
	replacement := prepare(t, repo, &prepareSpec{files: map[string]string{"data.txt": "v2"}, opID: "op-2"})
	fault.hook = crashAt(faultAfterMetadata)
	_, err := repo.Publish(context.Background(), replacement, artifact.PublishOptions{Force: true, ExpectedPrior: &old})
	require.ErrorIs(t, err, errCrash)

	_, err = repo.Delete(context.Background(), old, artifact.DeleteOptions{Scope: artifact.DeleteObject})
	require.Error(t, err, "a pending operation is never deleted out from under its owner")
	require.ErrorContains(t, err, "op-2")
}

// A payload location replaced by a symlink to a directory outside the root must not let a delete
// reach the outside files: every operation is confined to the repository root.
func TestDelete_NeverEscapesTheRootThroughASymlink(t *testing.T) {
	fixtures.RequirePOSIXModes(t)
	repo := newRepo(t, dirSpec())
	result, _ := publishFilePackage(t, repo)
	pinned := resolve(t, repo, defaultRef())

	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "README.md"), []byte("outside readme"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "empty.txt"), nil, 0o644))
	require.NoError(t, os.RemoveAll(result.Location))
	if err := os.Symlink(outside, result.Location); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	_, err := repo.Delete(context.Background(), pinned, artifact.DeleteOptions{Scope: artifact.DeleteObject})
	require.Error(t, err, "the swapped location cannot be verified or removed")
	assert.FileExists(t, filepath.Join(outside, "README.md"))
	assert.FileExists(t, filepath.Join(outside, "empty.txt"))
}

// A stored representation that names a location outside the root is rejected on load.
func TestStoredLocationOutsideTheRootIsRejected(t *testing.T) {
	repo := newRepo(t, dirSpec())
	publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
	pinned := resolve(t, repo, defaultRef())
	key := layout.ReferenceKey(testName, testVersion)

	tr, err := repo.openTree(false)
	require.NoError(t, err)
	rep, err := tr.loadRepresentation(key)
	require.NoError(t, err)
	require.NoError(t, tr.Close())

	rep.Location = "../escape"
	_, marshalErr := rep.Marshal()
	require.Error(t, marshalErr, "the layout refuses to serialize an escaping location")

	// Write the document by hand to simulate tampering.
	raw := `{"documents":[],"location":"../escape","name":"team/app","operation_id":"x","packaging":"none","payload":[],"record_digest":"` + pinned.RecordDigest.String() + `","repository_kind":"local/dir","schema_version":1,"version":"1.0.0"}`
	require.NoError(t, os.WriteFile(filepath.Join(repo.root, filepath.FromSlash(layout.RepresentationPath(key))), []byte(raw), 0o644))
	_, err = repo.Inspect(context.Background(), pinned)
	require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	_, err = repo.Delete(context.Background(), pinned, artifact.DeleteOptions{Scope: artifact.DeleteObject})
	require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
}

func TestList_EnumeratesManagedReferencesOnly(t *testing.T) {
	for _, spec := range allSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			var fault armedFault
			spec = spec.withOpts(fault.option())
			repo := newRepo(t, spec)
			for _, item := range []struct{ name, version string }{{"b/app", "1"}, {"a/app", "2"}, {"a/app", "1"}} {
				publish(t, repo, &prepareSpec{name: item.name, version: item.version, labels: map[string]string{"team": item.name}}, artifact.PublishOptions{})
			}
			// Unmanaged files, a stray directory, and a pending publication are all ignored.
			require.NoError(t, os.WriteFile(filepath.Join(repo.root, "user.txt"), []byte("x"), 0o644))
			require.NoError(t, os.MkdirAll(filepath.Join(repo.root, "a", "stray"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(repo.root, "a", "stray", "file.json"), []byte("{}"), 0o644))
			pending := prepare(t, repo, &prepareSpec{name: "c/pending", opID: "op-pending"})
			fault.hook = crashAt(faultAfterMetadata)
			_, err := repo.Publish(context.Background(), pending, artifact.PublishOptions{})
			require.ErrorIs(t, err, errCrash)

			assert.Equal(t, []string{"a/app:1", "a/app:2", "b/app:1"}, listNames(t, repo))

			named, err := repo.List(context.Background(), artifact.ListQuery{Name: "a/app"})
			require.NoError(t, err)
			require.Len(t, named.Entries, 2)
			assert.Equal(t, "1", named.Entries[0].Version)
			assert.Equal(t, "2", named.Entries[1].Version)

			labeled, err := repo.List(context.Background(), artifact.ListQuery{Labels: []artifact.Label{{Key: "team", Value: "b/app"}}})
			require.NoError(t, err)
			require.Len(t, labeled.Entries, 1)
			assert.Equal(t, "b/app", labeled.Entries[0].Name)
			assert.Equal(t, artifact.KindFiles, labeled.Entries[0].Kind)
			assert.Equal(t, repo.root, labeled.Entries[0].Pinned.Repository)
		})
	}
}

func TestList_PaginatesInLexicalOrder(t *testing.T) {
	repo := newRepo(t, dirSpec())
	for _, item := range []struct{ name, version string }{{"b/app", "1"}, {"a/app", "10"}, {"a/app", "2"}, {"a/app-x", "1"}, {"c/app", "1"}} {
		publish(t, repo, &prepareSpec{name: item.name, version: item.version}, artifact.PublishOptions{})
	}
	var seen []string
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		page, err := repo.List(context.Background(), artifact.ListQuery{PageSize: 2, Cursor: cursor})
		require.NoError(t, err)
		for index := range page.Entries {
			seen = append(seen, page.Entries[index].Name+":"+page.Entries[index].Version)
		}
		if page.NextCursor == "" {
			break
		}
		require.NotEqual(t, cursor, page.NextCursor)
		cursor = page.NextCursor
	}
	assert.Equal(t, []string{"a/app:10", "a/app:2", "a/app-x:1", "b/app:1", "c/app:1"}, seen, "name then version, every entry exactly once")
}

func TestList_EmptyOrMissingRoot(t *testing.T) {
	repo := newRepo(t, dirSpec())
	page, err := repo.List(context.Background(), artifact.ListQuery{})
	require.NoError(t, err)
	assert.Empty(t, page.Entries)
	_, statErr := os.Stat(repo.root)
	assert.True(t, os.IsNotExist(statErr), "listing never creates the root")

	_, err = repo.Resolve(context.Background(), defaultRef())
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, statErr = os.Stat(repo.root)
	assert.True(t, os.IsNotExist(statErr), "reads never create the root")
}
