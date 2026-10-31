package s3

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// Listing enumerates managed completed references only: never arbitrary user objects, pending
// publications, journals, claims, or evidence.
func TestList_EnumeratesManagedReferencesOnly(t *testing.T) {
	for _, spec := range []*rspec{noneSpec(), archiveSpec(artifact.PackagingTgz)} {
		t.Run(specName(spec), func(t *testing.T) {
			spec = spec.fresh(t).with(func(s *rspec) { s.prefix = testPrefix })
			var fault armedFault
			repo := newRepo(t, spec.withOpts(fault.option()))
			publish(t, repo, &prepareSpec{name: "a/app", version: "1"}, artifact.PublishOptions{})
			publish(t, repo, &prepareSpec{name: "a/app", version: "2"}, artifact.PublishOptions{})
			fault.hook = crashAt(faultAfterMetadata)
			_, err := repo.Publish(context.Background(), prepare(t, repo, &prepareSpec{name: "b/pending", version: "1"}), artifact.PublishOptions{})
			require.ErrorIs(t, err, errCrash)
			spec.fake.Seed(testPrefix+"/user/notes.json", []byte(`{"name":"not-an-artifact"}`), nil)
			spec.fake.Seed(testPrefix+"/"+layout.ReferencesDir()+"/nested/ignored.json", []byte(`{}`), nil)
			spec.fake.Seed(testPrefix+"/"+layout.ReferencesDir()+"/notes.txt", []byte(`x`), nil)

			assert.Equal(t, []string{"a/app:1", "a/app:2"}, listNames(t, repo))
		})
	}
}

func TestList_PaginatesInLexicalOrderAndFiltersByLabel(t *testing.T) {
	repo := newRepo(t, noneSpec())
	for _, item := range []struct{ name, version, tier string }{{"b/app", "1", "gold"}, {"a/app", "2", "gold"}, {"a/app", "1", "silver"}, {"c/app", "1", "gold"}, {"a/app", "10", "gold"}} {
		publish(t, repo, &prepareSpec{name: item.name, version: item.version, labels: map[string]string{"tier": item.tier}}, artifact.PublishOptions{})
	}
	var seen []string
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		page, err := repo.List(context.Background(), artifact.ListQuery{Cursor: cursor, PageSize: 2})
		require.NoError(t, err)
		assert.LessOrEqual(t, len(page.Entries), 2)
		for index := range page.Entries {
			seen = append(seen, page.Entries[index].Name+":"+page.Entries[index].Version)
		}
		if page.NextCursor == "" {
			break
		}
		require.NotEqual(t, cursor, page.NextCursor)
		cursor = page.NextCursor
	}
	assert.Equal(t, []string{"a/app:1", "a/app:10", "a/app:2", "b/app:1", "c/app:1"}, seen, "lexical order, every entry exactly once")

	page, err := repo.List(context.Background(), artifact.ListQuery{Labels: []artifact.Label{{Key: "tier", Value: "gold"}}, PageSize: 2})
	require.NoError(t, err)
	require.Len(t, page.Entries, 2)
	assert.Equal(t, "a/app:10", page.Entries[0].Name+":"+page.Entries[0].Version)
	assert.NotEmpty(t, page.NextCursor, "the label filter pages too")

	named, err := repo.List(context.Background(), artifact.ListQuery{Name: "a/app"})
	require.NoError(t, err)
	assert.Len(t, named.Entries, 3)

	empty, err := newRepo(t, noneSpec()).List(context.Background(), artifact.ListQuery{})
	require.NoError(t, err)
	assert.Empty(t, empty.Entries, "an empty bucket lists nothing")
}

// More references than one S3 listing page hold are all found, because the listing follows the
// continuation token; the fake caps pages well below the client's request.
func TestList_FollowsContinuationAcrossManyReferences(t *testing.T) {
	const count, cap = 1010, 100
	spec := noneSpec().with(func(s *rspec) { s.fake = s3test.NewServer(t, s3test.WithPageCap(cap)); s.prefix = testPrefix })
	repo := newRepo(t, spec)
	base := publish(t, repo, &prepareSpec{name: "seed/app", version: "0"}, artifact.PublishOptions{})
	template, _, err := repo.loadRecord(context.Background(), base.Pinned.RecordDigest)
	require.NoError(t, err)
	seedReferences(t, repo, template, count)
	spec.fake.ResetRequests()

	var versions []string
	cursor := ""
	pages := 0
	for ; pages < 20; pages++ {
		page, err := repo.List(context.Background(), artifact.ListQuery{Name: "seed/app", Cursor: cursor, PageSize: 300})
		require.NoError(t, err)
		for index := range page.Entries {
			versions = append(versions, page.Entries[index].Version)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	require.Len(t, versions, count+1, "the original and every seeded reference")
	assert.Equal(t, "0", versions[0])
	assert.Equal(t, "v0000", versions[1])
	assert.Equal(t, "v1009", versions[len(versions)-1])
	assert.True(t, sort.StringsAreSorted(versions), "sorted lexically across page boundaries")
	refListings := 0
	for _, request := range spec.fake.RequestsFor(s3test.OpListObjectsV2) {
		if strings.Contains(request.Query.Get("prefix"), "/refs/") {
			refListings++
		}
	}
	assert.GreaterOrEqual(t, refListings, (pages+1)*((count+1)/cap), "every List walked every S3 page of the reference objects")
}

// seedReferences writes n completed references for versions v0000..v<n-1>, each with its own
// Record, straight into the bucket.
func seedReferences(t *testing.T, repo *Repository, template *artifact.Record, n int) {
	t.Helper()
	require.NoError(t, repo.forEach(context.Background(), n, func(ctx context.Context, index int) error {
		record := *template
		record.Version = fmt.Sprintf("v%04d", index)
		if err := record.Seal(); err != nil {
			return err
		}
		raw, err := record.Canonical()
		if err != nil {
			return err
		}
		recordDigest := digest.FromBytes(raw)
		intent, err := record.IntentDigest()
		if err != nil {
			return err
		}
		if err := repo.putBytes(ctx, layout.RecordPath(recordDigest), raw); err != nil {
			return err
		}
		completed := layout.Reference{
			SchemaVersion: layout.SchemaVersion, Name: record.Name, Version: record.Version, State: layout.StateCompleted,
			Generation: 2, OperationID: "seed", IntentDigest: intent, RecordDigest: recordDigest,
		}
		encoded, err := completed.Marshal()
		if err != nil {
			return err
		}
		return repo.putBytes(ctx, layout.ReferencePath(completed.Key()), encoded)
	}))
}
