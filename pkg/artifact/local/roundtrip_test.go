package local

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
	"github.com/cloudposse/atmos/pkg/artifact/materialize"
	"github.com/cloudposse/atmos/pkg/artifact/produce"
)

// publishFilePackage publishes the shared "File package" fixture: nested files, an executable
// helper, an empty file, an empty directory, a confined symlink, a Unicode filename, and distinct
// package README and CHANGELOG documents.
func publishFilePackage(t *testing.T, repo *Repository) (*artifact.PublishResult, fixtures.FilePackage) {
	t.Helper()
	src := t.TempDir()
	pkg := fixtures.WriteFilePackage(t, src)
	prepared, err := produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), artifact.PrepareRequest{
		Source: src, Name: testName, Version: testVersion, Packaging: repo.packaging,
		OperationID: "op-file-package", PreparedAt: capturedAt,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = prepared.Close() })
	result, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
	require.NoError(t, err)
	return result, pkg
}

func TestRoundTrip_EveryAllowedPackaging(t *testing.T) {
	specs := []*repoSpec{
		dirSpec(),
		archiveSpec(artifact.PackagingTar),
		archiveSpec(artifact.PackagingTgz),
		archiveSpec(artifact.PackagingZip),
	}
	for _, spec := range specs {
		name := spec.kind + "/" + string(spec.packaging)
		t.Run(name, func(t *testing.T) {
			repo := newRepo(t, spec)
			result, pkg := publishFilePackage(t, repo)
			assert.Equal(t, artifact.OutcomeCreated, result.Outcome)
			assert.Equal(t, artifact.LockScopeHost, result.LockScope)
			assert.Equal(t, repo.packaging, result.Representation.Packaging)

			pinned := resolve(t, repo, defaultRef())
			assert.Equal(t, pinned.RecordDigest, pinned.ResolvedDigest)
			assert.Equal(t, result.Pinned.RecordDigest, pinned.RecordDigest)

			content, err := repo.Read(context.Background(), pinned)
			require.NoError(t, err)
			defer content.Close()
			out := filepath.Join(t.TempDir(), "out")
			require.NoError(t, materialize.Materialize(context.Background(), content, out))
			state, err := materialize.InspectDestination(out, content.Record)
			require.NoError(t, err)
			assert.Equal(t, materialize.DestinationMatches, state, "the pulled tree matches the inventory, modes included")

			assert.Equal(t, fixtures.FilePackagePaths(pkg.Symlinks), listInventoryPaths(content.Record))
			assert.NoDirExists(t, filepath.Join(out, ".atmos"), "the distribution directory never leaks into pulled user content")
			assert.FileExists(t, filepath.Join(out, "README.md"), "a root README is ordinary payload")
			fixtures.RequirePOSIXModes(t)
			info, err := os.Stat(filepath.Join(out, "bin", "helper.sh"))
			require.NoError(t, err)
			assert.NotZero(t, info.Mode()&0o111, "the executable bit survives the round trip")
		})
	}
}

func listInventoryPaths(record *artifact.Record) []string {
	paths := make([]string, 0, len(record.Inventory))
	for index := range record.Inventory {
		paths = append(paths, record.Inventory[index].Path)
	}
	sort.Strings(paths)
	return paths
}

func TestDirLayout_PayloadIsOrdinaryFilesWithAdjacentDistribution(t *testing.T) {
	repo := newRepo(t, dirSpec())
	result, _ := publishFilePackage(t, repo)
	location := filepath.Join(repo.root, "team", "app", "1.0.0")
	assert.Equal(t, location, result.Location)

	assert.FileExists(t, filepath.Join(location, "README.md"))
	data, err := os.ReadFile(filepath.Join(location, "nested", "deep", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "deep content\n", string(data), "payload files are ordinary copied files")
	assert.DirExists(t, filepath.Join(location, "emptydir"))

	dist := filepath.Join(location, ".atmos", "artifact")
	for name, want := range map[string]string{"README.md": fixtures.PackageReadme, "CHANGELOG.md": fixtures.PackageChangelog} {
		got, err := os.ReadFile(filepath.Join(dist, name))
		require.NoError(t, err)
		assert.Equal(t, want, string(got))
	}
	manifest, err := os.ReadFile(filepath.Join(dist, "manifest.json"))
	require.NoError(t, err)
	record, err := artifact.DecodeRecord(manifest)
	require.NoError(t, err)
	assert.Equal(t, testName, record.Name)

	for _, managed := range []string{
		layout.ReferencePath(layout.ReferenceKey(testName, testVersion)),
		layout.RepresentationPath(layout.ReferenceKey(testName, testVersion)),
		layout.RecordPath(result.Pinned.RecordDigest),
	} {
		assert.FileExists(t, filepath.Join(repo.root, filepath.FromSlash(managed)))
	}
	assert.NoDirExists(t, filepath.Join(repo.root, filepath.FromSlash(layout.StagingPath("op-file-package"))), "staging is cleaned")
	assert.NoFileExists(t, filepath.Join(repo.root, filepath.FromSlash(layout.OperationPath("op-file-package"))), "the finished journal is removed")
}

func TestArchiveLayout_SidecarsAdjacentAndPortableCopiesEmbedded(t *testing.T) {
	repo := newRepo(t, archiveSpec(artifact.PackagingTgz))
	result, _ := publishFilePackage(t, repo)
	archivePath := filepath.Join(repo.root, "team", "app", "1.0.0.tgz")
	assert.Equal(t, archivePath, result.Location)

	assert.FileExists(t, archivePath)
	metadata, err := os.ReadFile(archivePath + ".metadata.json")
	require.NoError(t, err)
	record, err := artifact.DecodeRecord(metadata)
	require.NoError(t, err)
	assert.Equal(t, result.Pinned.ContentDigest, record.ContentDigest)
	for name, want := range map[string]string{"README.md": fixtures.PackageReadme, "CHANGELOG.md": fixtures.PackageChangelog} {
		got, err := os.ReadFile(filepath.Join(archivePath+".docs", name))
		require.NoError(t, err)
		assert.Equal(t, want, string(got))
	}

	members := tgzMembers(t, archivePath)
	assert.Contains(t, members, ".atmos/artifact/manifest.json", "the archive embeds a portable manifest")
	assert.Contains(t, members, ".atmos/artifact/README.md")
	assert.Equal(t, string(metadata), members[".atmos/artifact/manifest.json"], "the embedded manifest equals the adjacent metadata")
}

// tgzMembers returns file member contents by name.
func tgzMembers(t *testing.T, archivePath string) map[string]string {
	t.Helper()
	file, err := os.Open(archivePath)
	require.NoError(t, err)
	defer file.Close()
	gz, err := gzip.NewReader(file)
	require.NoError(t, err)
	members := map[string]string{}
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if header.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(reader)
			require.NoError(t, err)
			members[header.Name] = string(data)
		}
	}
	return members
}

func TestDocumentsAreReadWithoutThePayload(t *testing.T) {
	for _, spec := range []*repoSpec{dirSpec(), archiveSpec(artifact.PackagingTgz)} {
		t.Run(spec.kind, func(t *testing.T) {
			repo := newRepo(t, spec)
			result, _ := publishFilePackage(t, repo)
			pinned := resolve(t, repo, defaultRef())

			// Make the main payload unreadable: documents and metadata must not need it.
			if spec.kind == artifact.RepositoryKindLocalArchive {
				require.NoError(t, os.Remove(result.Location))
			} else {
				require.NoError(t, os.Remove(filepath.Join(result.Location, "README.md")))
				require.NoError(t, os.Remove(filepath.Join(result.Location, "nested", "deep", "file.txt")))
			}

			stream, err := repo.OpenDocument(context.Background(), pinned, artifact.DocumentChangelog)
			require.NoError(t, err)
			data, err := io.ReadAll(stream.Reader)
			require.NoError(t, err)
			require.NoError(t, stream.Reader.Close())
			assert.Equal(t, fixtures.PackageChangelog, string(data))

			inspection, err := repo.Inspect(context.Background(), pinned)
			require.NoError(t, err)
			assert.Equal(t, testName, inspection.Record.Name)

			_, err = repo.OpenDocument(context.Background(), pinned, "unknown-role")
			require.ErrorIs(t, err, errUtils.ErrArtifactDocumentMissing)

			content, err := repo.Read(context.Background(), pinned)
			if err == nil {
				defer content.Close()
				err = readAll(t, content)
			}
			require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed, "the payload itself is verified and reports the loss")
		})
	}
}

func TestPullReplacementRemovesStaleFilesInsideTheOutputOnly(t *testing.T) {
	for _, spec := range []*repoSpec{dirSpec(), archiveSpec(artifact.PackagingTgz)} {
		t.Run(spec.kind, func(t *testing.T) {
			repo := newRepo(t, spec)
			publishFilePackage(t, repo)
			pinned := resolve(t, repo, defaultRef())

			parent := t.TempDir()
			out := filepath.Join(parent, "out")
			require.NoError(t, os.MkdirAll(filepath.Join(out, "stale"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(out, "stale", "old.txt"), []byte("old"), 0o644))
			require.NoError(t, os.WriteFile(filepath.Join(out, "README.md"), []byte("old readme"), 0o644))
			sibling := filepath.Join(parent, "sibling.txt")
			require.NoError(t, os.WriteFile(sibling, []byte("keep"), 0o644))

			content, err := repo.Read(context.Background(), pinned)
			require.NoError(t, err)
			defer content.Close()
			state, err := materialize.InspectDestination(out, content.Record)
			require.NoError(t, err)
			assert.NotEqual(t, materialize.DestinationMatches, state)
			require.NoError(t, materialize.Materialize(context.Background(), content, out))

			assert.NoFileExists(t, filepath.Join(out, "stale", "old.txt"), "stale files inside the output are removed, not merged")
			assert.FileExists(t, sibling, "nothing outside the requested output root is touched")
			got, err := os.ReadFile(filepath.Join(out, "README.md"))
			require.NoError(t, err)
			assert.Equal(t, fixtures.RootReadme, string(got))
		})
	}
}

func TestSafeRelativeSymlinkRoundTrips(t *testing.T) {
	for _, spec := range []*repoSpec{dirSpec(), archiveSpec(artifact.PackagingTar)} {
		t.Run(spec.kind, func(t *testing.T) {
			repo := newRepo(t, spec)
			src := t.TempDir()
			fixtures.WriteFile(t, src, "real/target.txt", "target\n", 0o644)
			if !fixtures.Symlink(t, src, "alias.txt", "real/target.txt") {
				t.Skip("symlinks are unavailable on this host")
			}
			prepared, err := produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), artifact.PrepareRequest{
				Source: src, Name: testName, Version: testVersion, Packaging: repo.packaging, OperationID: "op-link", PreparedAt: capturedAt,
			})
			require.NoError(t, err)
			defer prepared.Close()
			_, err = repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
			require.NoError(t, err)

			content, err := repo.Read(context.Background(), resolve(t, repo, defaultRef()))
			require.NoError(t, err)
			defer content.Close()
			out := filepath.Join(t.TempDir(), "out")
			require.NoError(t, materialize.Materialize(context.Background(), content, out))
			target, err := os.Readlink(filepath.Join(out, "alias.txt"))
			require.NoError(t, err)
			assert.Equal(t, filepath.FromSlash("real/target.txt"), target, "a contained relative link stays a link")
		})
	}
}

func TestExecutableBitsSurviveAsOrdinaryFiles(t *testing.T) {
	fixtures.RequirePOSIXModes(t)
	repo := newRepo(t, dirSpec())
	result, _ := publishFilePackage(t, repo)
	info, err := os.Stat(filepath.Join(result.Location, "bin", "helper.sh"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), info.Mode().Perm(), "the stored payload keeps the ordinary executable mode")
	info, err = os.Stat(filepath.Join(result.Location, fixtures.UnicodeName))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
	assert.True(t, repo.Capabilities().PreservesModes)
}
