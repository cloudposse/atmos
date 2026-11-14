package s3

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
	"github.com/cloudposse/atmos/pkg/artifact/materialize"
)

// A published package materializes back with every inventory entry, mode, empty file and
// directory, symlink, and Unicode name intact, in every packaging.
func TestRoundTrip_EveryPackaging(t *testing.T) {
	for _, spec := range allSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			spec = spec.fresh(t)
			repo := newRepo(t, spec)
			prep, pkg := packageSpec(t)
			result := publish(t, repo, prep, artifact.PublishOptions{})
			content, err := repo.Read(context.Background(), result.Pinned)
			require.NoError(t, err)
			defer content.Close()

			out := filepath.Join(t.TempDir(), "out")
			require.NoError(t, materialize.Materialize(context.Background(), content, out))
			state, err := materialize.InspectDestination(out, content.Record)
			require.NoError(t, err)
			assert.Equal(t, materialize.DestinationMatches, state)
			assert.Equal(t, fixtures.FilePackagePaths(pkg.Symlinks), inventoryPaths(content.Record))
		})
	}
}

func inventoryPaths(record *artifact.Record) []string {
	paths := make([]string, 0, len(record.Inventory))
	for index := range record.Inventory {
		paths = append(paths, record.Inventory[index].Path)
	}
	return paths
}

// Per-file packaging is plain objects below the location with the distribution beside them; an
// archive is one object with metadata and document objects beside it.
func TestObjectLayout(t *testing.T) {
	t.Run("files", func(t *testing.T) {
		spec := noneSpec().fresh(t).with(func(s *rspec) { s.prefix = testPrefix })
		repo := newRepo(t, spec)
		prep, pkg := packageSpec(t)
		result := publish(t, repo, prep, artifact.PublishOptions{})
		assert.Equal(t, "s3://"+spec.fake.Bucket()+"/"+testPrefix+"/team/app/1.0.0", result.Location)

		want := []string{
			"team/app/1.0.0/.atmos/artifact/CHANGELOG.md",
			"team/app/1.0.0/.atmos/artifact/README.md",
			"team/app/1.0.0/.atmos/artifact/manifest.json",
			"team/app/1.0.0/README.md",
			"team/app/1.0.0/bin/helper.sh",
			"team/app/1.0.0/empty.txt",
			"team/app/1.0.0/nested/deep/file.txt",
			"team/app/1.0.0/" + fixtures.UnicodeName,
		}
		assert.Equal(t, want, payloadKeys(spec.fake, testPrefix), "directories and symlinks live in the Record, not as objects (symlinks: %v)", pkg.Symlinks)
		empty, ok := spec.fake.Object(testPrefix + "/team/app/1.0.0/empty.txt")
		require.True(t, ok)
		assert.Zero(t, empty.Size, "zero-length files are stored as zero-length objects")
	})
	t.Run("archive", func(t *testing.T) {
		spec := archiveSpec(artifact.PackagingTgz).fresh(t).with(func(s *rspec) { s.prefix = testPrefix })
		repo := newRepo(t, spec)
		prep, _ := packageSpec(t)
		result := publish(t, repo, prep, artifact.PublishOptions{})
		assert.Equal(t, "s3://"+spec.fake.Bucket()+"/"+testPrefix+"/team/app/1.0.0.tgz", result.Location)
		assert.Equal(t, []string{
			"team/app/1.0.0.tgz",
			"team/app/1.0.0.tgz.docs/CHANGELOG.md",
			"team/app/1.0.0.tgz.docs/README.md",
			"team/app/1.0.0.tgz.metadata.json",
		}, payloadKeys(spec.fake, testPrefix))
		stored, ok := spec.fake.Object(testPrefix + "/team/app/1.0.0.tgz")
		require.True(t, ok)
		assert.Equal(t, result.Representation.Payload[0].Digest, digest.FromBytes(stored.Data), "the representation records the archive digest, not the ETag")
		assert.NotEqual(t, stored.ETag, result.Representation.Payload[0].Digest.String())
	})
}

// Documents are readable without opening the payload.
func TestDocumentsAreReadWithoutThePayload(t *testing.T) {
	for _, spec := range allSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			spec = spec.fresh(t)
			repo := newRepo(t, spec)
			prep, _ := packageSpec(t)
			result := publish(t, repo, prep, artifact.PublishOptions{})
			spec.fake.ResetRequests()

			stream, err := repo.OpenDocument(context.Background(), result.Pinned, artifact.DocumentChangelog)
			require.NoError(t, err)
			data, err := io.ReadAll(stream.Reader)
			require.NoError(t, err)
			assert.Equal(t, fixtures.PackageChangelog, string(data))
			for _, request := range spec.fake.Requests() {
				managed := layout.InManagedNamespace(request.Key)
				assert.True(t, managed || request.Key == documentKeyFor(spec.packaging, "CHANGELOG.md"),
					"only the document object and managed records are requested: %s %s", request.Op, request.Key)
			}
			inspection, err := repo.Inspect(context.Background(), result.Pinned)
			require.NoError(t, err)
			assert.Equal(t, spec.packaging, inspection.Representation.Packaging)
		})
	}
}

// Multipart transfers keep memory bounded and honor the configured part size and concurrency.
func TestPublish_LargePayloadUsesBoundedMultipart(t *testing.T) {
	spec := noneSpec().fresh(t).withOpts(WithConcurrency(2))
	repo := newRepo(t, spec)
	publish(t, repo, &prepareSpec{files: map[string]string{"big.bin": incompressible(256 << 10)}}, artifact.PublishOptions{})
	assert.GreaterOrEqual(t, spec.fake.CompletedUploads(), 1)
	assert.Empty(t, spec.fake.OpenUploads())
	assert.LessOrEqual(t, spec.fake.MaxConcurrentParts(), 4, "uploads stay bounded")
	assertIntact(t, repo, defaultRef())
}
