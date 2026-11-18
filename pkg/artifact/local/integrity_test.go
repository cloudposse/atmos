package local

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// tamper rewrites one file below the repository root after applying edit to its bytes.
func tamper(t *testing.T, root, rel string, edit func([]byte) []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	original, err := os.ReadFile(path)
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(path, 0o644))
	require.NoError(t, os.WriteFile(path, edit(original), info.Mode().Perm()))
}

func flipFirstByte(data []byte) []byte {
	out := append([]byte(nil), data...)
	out[0] ^= 0xff
	return out
}

func appendJunk(data []byte) []byte {
	return append(append([]byte(nil), data...), []byte("junk")...)
}

func truncate(data []byte) []byte { return data[:len(data)/2] }

// integrityCase names one corruption and where it surfaces.
type integrityCase struct {
	name string
	// target returns the file to corrupt, relative to the root, given the stored location.
	target func(location string, record *artifact.Record) string
	edit   func([]byte) []byte
	// surfaces lists the operations that must fail with an integrity error.
	surfaces []string
}

func TestTamperedState_FailsIntegrity(t *testing.T) {
	ref := defaultRef()
	managed := func(path func(key string, recordDigest string) string) func(string, *artifact.Record) string {
		return func(_ string, record *artifact.Record) string {
			digestValue, err := record.RecordDigest()
			require.NoError(t, err)
			return path(layout.ReferenceKey(record.Name, record.Version), digestValue.Encoded())
		}
	}
	cases := []integrityCase{
		{
			name: "record file bytes",
			target: managed(func(_, recordHex string) string {
				return ".atmos-artifacts/v1/records/" + recordHex + ".json"
			}),
			edit: appendJunk, surfaces: []string{"resolve", "list"},
		},
		{
			name:     "reference file garbage",
			target:   managed(func(key, _ string) string { return layout.ReferencePath(key) }),
			edit:     truncate,
			surfaces: []string{"resolve", "list"},
		},
		{
			name:     "representation file",
			target:   managed(func(key, _ string) string { return layout.RepresentationPath(key) }),
			edit:     flipFirstByte,
			surfaces: []string{"inspect", "read", "document"},
		},
	}
	for _, spec := range allSpecs {
		for _, tc := range cases {
			t.Run(specName(spec)+"/"+tc.name, func(t *testing.T) {
				repo := newRepo(t, spec)
				result, _ := publishFilePackage(t, repo)
				pinned := resolve(t, repo, ref)
				inspection, err := repo.Inspect(context.Background(), pinned)
				require.NoError(t, err)
				tamper(t, repo.root, tc.target(result.Location, inspection.Record), tc.edit)
				assertIntegrityFailure(t, repo, ref, &pinned, tc.surfaces)
			})
		}
	}
}

func assertIntegrityFailure(t *testing.T, repo *Repository, r artifact.Ref, pinned *artifact.PinnedReference, surfaces []string) {
	t.Helper()
	ctx := context.Background()
	for _, surface := range surfaces {
		var err error
		switch surface {
		case "resolve":
			_, err = repo.Resolve(ctx, r)
		case "list":
			_, err = repo.List(ctx, artifact.ListQuery{})
		case "inspect":
			_, err = repo.Inspect(ctx, *pinned)
		case "read":
			_, err = repo.Read(ctx, *pinned)
		case "document":
			_, err = repo.OpenDocument(ctx, *pinned, artifact.DocumentReadme)
		}
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed, surface)
	}
}

func TestTamperedMetadataSidecar_FailsIntegrity(t *testing.T) {
	for _, tc := range []struct {
		spec     *repoSpec
		sidecar  func(location string) string
		describe string
	}{
		{archiveSpec(artifact.PackagingTgz), func(l string) string { return l + ".metadata.json" }, "archive metadata sidecar"},
		{dirSpec(), func(l string) string { return l + "/.atmos/artifact/manifest.json" }, "directory manifest"},
	} {
		t.Run(tc.describe, func(t *testing.T) {
			repo := newRepo(t, tc.spec)
			result, _ := publishFilePackage(t, repo)
			pinned := resolve(t, repo, defaultRef())
			rel := filepath.ToSlash(mustRel(t, repo.root, result.Location))
			tamper(t, repo.root, tc.sidecar(rel), appendJunk)

			_, err := repo.Inspect(context.Background(), pinned)
			require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
			_, err = repo.Read(context.Background(), pinned)
			require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
		})
	}
}

func mustRel(t *testing.T, base, target string) string {
	t.Helper()
	rel, err := filepath.Rel(base, target)
	require.NoError(t, err)
	return rel
}

func TestMissingSidecars_FailExplicitly(t *testing.T) {
	t.Run("archive metadata sidecar", func(t *testing.T) {
		repo := newRepo(t, archiveSpec(artifact.PackagingTgz))
		result, _ := publishFilePackage(t, repo)
		pinned := resolve(t, repo, defaultRef())
		require.NoError(t, os.Remove(result.Location+".metadata.json"))
		_, err := repo.Inspect(context.Background(), pinned)
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed, "a completed reference promised the sidecar exists")
		_, err = repo.Read(context.Background(), pinned)
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	})
	t.Run("archive document sidecar", func(t *testing.T) {
		repo := newRepo(t, archiveSpec(artifact.PackagingTgz))
		result, _ := publishFilePackage(t, repo)
		pinned := resolve(t, repo, defaultRef())
		require.NoError(t, os.Remove(filepath.Join(result.Location+".docs", "README.md")))
		_, err := repo.OpenDocument(context.Background(), pinned, artifact.DocumentReadme)
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
		assert.NotErrorIs(t, err, errUtils.ErrArtifactDocumentMissing, "a lost sidecar is damage, not an absent optional document")
		_, err = repo.OpenDocument(context.Background(), pinned, artifact.DocumentChangelog)
		require.NoError(t, err, "the other sidecar is unaffected")
	})
	t.Run("directory distribution", func(t *testing.T) {
		repo := newRepo(t, dirSpec())
		result, _ := publishFilePackage(t, repo)
		pinned := resolve(t, repo, defaultRef())
		require.NoError(t, os.Remove(filepath.Join(result.Location, ".atmos", "artifact", "CHANGELOG.md")))
		_, err := repo.OpenDocument(context.Background(), pinned, artifact.DocumentChangelog)
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	})
}

func TestTamperedPayloadAndDocuments_FailAtTheDeclaredDigest(t *testing.T) {
	t.Run("archive bytes", func(t *testing.T) {
		repo := newRepo(t, archiveSpec(artifact.PackagingTgz))
		result, _ := publishFilePackage(t, repo)
		pinned := resolve(t, repo, defaultRef())
		tamper(t, repo.root, filepath.ToSlash(mustRel(t, repo.root, result.Location)), flipFirstByteInPlace)
		content, err := repo.Read(context.Background(), pinned)
		require.NoError(t, err, "opening is cheap; verification happens as the bytes are consumed")
		defer content.Close()
		require.ErrorIs(t, readAll(t, content), errUtils.ErrArtifactIntegrityFailed)
	})
	t.Run("archive truncated", func(t *testing.T) {
		repo := newRepo(t, archiveSpec(artifact.PackagingZip))
		result, _ := publishFilePackage(t, repo)
		pinned := resolve(t, repo, defaultRef())
		tamper(t, repo.root, filepath.ToSlash(mustRel(t, repo.root, result.Location)), truncate)
		_, err := repo.Read(context.Background(), pinned)
		require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed, "a size mismatch is caught before any byte is read")
	})
	t.Run("directory file", func(t *testing.T) {
		repo := newRepo(t, dirSpec())
		result, _ := publishFilePackage(t, repo)
		pinned := resolve(t, repo, defaultRef())
		tamper(t, repo.root, filepath.ToSlash(mustRel(t, repo.root, filepath.Join(result.Location, "nested", "deep", "file.txt"))), sameLengthEdit)
		content, err := repo.Read(context.Background(), pinned)
		require.NoError(t, err)
		defer content.Close()
		require.ErrorIs(t, readAll(t, content), errUtils.ErrArtifactIntegrityFailed)
	})
	t.Run("document", func(t *testing.T) {
		for _, spec := range []*repoSpec{dirSpec(), archiveSpec(artifact.PackagingTgz)} {
			repo := newRepo(t, spec)
			result, _ := publishFilePackage(t, repo)
			pinned := resolve(t, repo, defaultRef())
			docPath := filepath.Join(result.Location, ".atmos", "artifact", "README.md")
			if spec.kind == artifact.RepositoryKindLocalArchive {
				docPath = filepath.Join(result.Location+".docs", "README.md")
			}
			tamper(t, repo.root, filepath.ToSlash(mustRel(t, repo.root, docPath)), sameLengthEdit)
			stream, err := repo.OpenDocument(context.Background(), pinned, artifact.DocumentReadme)
			require.NoError(t, err)
			_, err = io.Copy(io.Discard, stream.Reader)
			require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed, spec.kind)
		}
	})
}

func flipFirstByteInPlace(data []byte) []byte {
	out := append([]byte(nil), data...)
	out[len(out)/2] ^= 0xff
	return out
}

// sameLengthEdit changes content without changing its size, so only the digest can catch it.
func sameLengthEdit(data []byte) []byte {
	out := append([]byte(nil), data...)
	out[0] = 'X'
	if len(data) > 0 && data[0] == 'X' {
		out[0] = 'Y'
	}
	return out
}

func TestExtraFilesInsideAPayloadAreNotAdopted(t *testing.T) {
	fixtures.RequirePOSIXModes(t)
	repo := newRepo(t, dirSpec())
	result, _ := publishFilePackage(t, repo)
	pinned := resolve(t, repo, defaultRef())
	require.NoError(t, os.WriteFile(filepath.Join(result.Location, "injected.txt"), []byte("not in the inventory"), 0o644))

	content, err := repo.Read(context.Background(), pinned)
	require.NoError(t, err)
	defer content.Close()
	for index := range content.Streams {
		assert.NotEqual(t, "injected.txt", content.Streams[index].Path, "only inventory files are ever served")
	}
}
