package s3

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// withPattern returns a spec whose key_pattern is set.
func (s *rspec) withPattern(pattern string) *rspec {
	return s.with(func(c *rspec) { c.keyPattern = pattern })
}

func TestDatedRoot_LocationIsRenderedOnceAndPersisted(t *testing.T) {
	const pattern = `{{ .CreatedAt | date "2006/01/02" }}/{{ .Name }}/{{ .Version }}`
	spec := noneSpec().fresh(t).withPattern(pattern)
	repo := newRepo(t, spec)
	result := publish(t, repo, &prepareSpec{at: time.Date(2026, 10, 30, 23, 59, 59, 0, time.UTC)}, artifact.PublishOptions{})

	assert.Equal(t, "s3://"+spec.fake.Bucket()+"/2026/10/30/team/app/1.0.0", result.Location)
	assert.Contains(t, payloadKeys(spec.fake, ""), "2026/10/30/team/app/1.0.0/data.txt")
	rep, err := repo.loadRepresentation(context.Background(), layout.ReferenceKey(testName, testVersion))
	require.NoError(t, err)
	assert.Equal(t, "2026/10/30/team/app/1.0.0", rep.Location)

	// A later read uses the persisted location, whatever the pattern now says.
	reader := newRepo(t, spec.withPattern("elsewhere/{{ .Name }}"))
	assertIntact(t, reader, defaultRef())
}

// A force replacement that lands on a new location, even with a different packaging, retires the
// old payload objects and Record, and the old digest reports not found.
func TestForceReplacementAcrossLocationsRetiresTheOldPayload(t *testing.T) {
	tests := []struct {
		name         string
		oldSpec      *rspec
		newSpec      *rspec
		oldPackaging artifact.Packaging
		newPackaging artifact.Packaging
	}{
		{name: "files to files", oldSpec: noneSpec().withPattern("old/{{ .Name }}"), newSpec: noneSpec().withPattern("new/{{ .Name }}")},
		{name: "files to archive", oldSpec: noneSpec().withPattern("old/{{ .Name }}"), newSpec: archiveSpec(artifact.PackagingTgz).withPattern("new/{{ .Name }}.tgz")},
		{name: "archive to files", oldSpec: archiveSpec(artifact.PackagingZip).withPattern("old/{{ .Name }}.zip"), newSpec: noneSpec().withPattern("new/{{ .Name }}")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := s3test.NewServer(t)
			oldRepo := newRepo(t, tt.oldSpec.with(func(s *rspec) { s.fake = fake }))
			newRepoUnderTest := newRepo(t, tt.newSpec.with(func(s *rspec) { s.fake = fake }))
			base := publish(t, oldRepo, &prepareSpec{files: map[string]string{"data.txt": "old", "extra.txt": "x"}}, artifact.PublishOptions{})
			oldKeys := payloadKeys(fake, "")
			require.NotEmpty(t, oldKeys)

			replaced := publish(t, newRepoUnderTest, &prepareSpec{files: map[string]string{"data.txt": "new"}}, artifact.PublishOptions{Force: true, ExpectedPrior: &base.Pinned})
			assert.Equal(t, artifact.OutcomeReplaced, replaced.Outcome)
			for _, key := range payloadKeys(fake, "") {
				assert.NotContains(t, key, "old/", "the old payload objects were retired: %s", key)
			}
			_, err := newRepoUnderTest.Read(context.Background(), base.Pinned)
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
			assertIntact(t, newRepoUnderTest, defaultRef())
		})
	}
}

// A second reference whose key_pattern resolves onto an owned location fails with a conflict and is
// never bound to the first reference's payload.
func TestCollidingKeyPattern_SecondReferenceConflicts(t *testing.T) {
	tests := []struct {
		name    string
		first   *rspec
		second  *rspec
		wantMsg string
	}{
		{name: "identical files location", first: noneSpec().withPattern("shared/payload"), second: noneSpec().withPattern("shared/payload")},
		{name: "identical archive location", first: archiveSpec(artifact.PackagingTgz).withPattern("shared/p.tgz"), second: archiveSpec(artifact.PackagingTgz).withPattern("shared/p.tgz")},
		{name: "second is below the first", first: noneSpec().withPattern("shared/payload"), second: noneSpec().withPattern("shared/payload/deeper")},
		{name: "second is above the first", first: noneSpec().withPattern("shared/payload/deeper"), second: noneSpec().withPattern("shared/payload")},
		{name: "case aliases", first: noneSpec().withPattern("shared/Payload"), second: noneSpec().withPattern("shared/payload")},
		{name: "second archive claims the first's metadata sidecar", first: archiveSpec(artifact.PackagingTgz).withPattern("shared/p.tgz"), second: archiveSpec(artifact.PackagingTgz).withPattern("shared/p.tgz.metadata.json")},
		{name: "second archive claims the first's documents directory", first: archiveSpec(artifact.PackagingTgz).withPattern("shared/p.tgz"), second: noneSpec().withPattern("shared/p.tgz.docs")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := s3test.NewServer(t)
			first := newRepo(t, tt.first.with(func(s *rspec) { s.fake = fake }))
			second := newRepo(t, tt.second.with(func(s *rspec) { s.fake = fake }))
			publish(t, first, &prepareSpec{name: "one/app", files: map[string]string{"a.txt": "first"}}, artifact.PublishOptions{})
			before := bucketKeys(fake)

			_, err := second.Publish(context.Background(), prepare(t, second, &prepareSpec{name: "two/app", files: map[string]string{"a.txt": "second"}}), artifact.PublishOptions{})
			require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
			assert.Equal(t, before, bucketKeys(fake), "the failed reservation changed nothing")
			assertIntact(t, first, ref("one/app", testVersion))
			_, err = second.Resolve(context.Background(), ref("two/app", testVersion))
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
		})
	}
}

// A pending publication reserves its location too.
func TestCollidingKeyPattern_PendingReservationBlocksOthers(t *testing.T) {
	fake := s3test.NewServer(t)
	first := newRepo(t, noneSpec().withPattern("shared/payload").with(func(s *rspec) { s.fake = fake }).withOpts(withFaults(crashAt(faultDuringPayload))))
	second := newRepo(t, noneSpec().withPattern("shared/payload").with(func(s *rspec) { s.fake = fake }))
	_, err := first.Publish(context.Background(), prepare(t, first, &prepareSpec{name: "one/app"}), artifact.PublishOptions{})
	require.ErrorIs(t, err, errCrash)

	_, err = second.Publish(context.Background(), prepare(t, second, &prepareSpec{name: "two/app"}), artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	require.ErrorContains(t, err, "op-one/app", "the pending operation is named")
}

// Objects the repository does not manage are never overwritten or adopted.
func TestUnmanagedContentAtTheLocationIsNeverOverwritten(t *testing.T) {
	tests := []struct {
		name string
		spec *rspec
		seed string
	}{
		{name: "file below the payload prefix", spec: noneSpec(), seed: "team/app/1.0.0/user.txt"},
		{name: "object at the payload key itself", spec: noneSpec(), seed: "team/app/1.0.0"},
		{name: "archive object", spec: archiveSpec(artifact.PackagingTgz), seed: "team/app/1.0.0.tgz"},
		{name: "object in the documents directory", spec: archiveSpec(artifact.PackagingTgz), seed: "team/app/1.0.0.tgz.docs/notes.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.spec.fresh(t)
			repo := newRepo(t, spec)
			spec.fake.Seed(tt.seed, []byte("mine"), nil)

			_, err := repo.Publish(context.Background(), prepare(t, repo, &prepareSpec{}), artifact.PublishOptions{})
			require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
			object, ok := spec.fake.Object(tt.seed)
			require.True(t, ok)
			assert.Equal(t, []byte("mine"), object.Data, "the user's object is untouched")
		})
	}
}

// Writers racing for one location: exactly one reference wins and the rest conflict.
func TestCollidingKeyPattern_ConcurrentWritersExactlyOneWins(t *testing.T) {
	const writers = 6
	fake := s3test.NewServer(t)
	repos := make([]*Repository, writers)
	prepared := make([]*artifact.PreparedArtifact, writers)
	for index := range repos {
		repos[index] = newRepo(t, noneSpec().withPattern("shared/payload").with(func(s *rspec) { s.fake = fake }).withOpts(WithLockWait(time.Minute)))
		prepared[index] = prepare(t, repos[index], &prepareSpec{name: fmt.Sprintf("writer%d/app", index), files: map[string]string{"a.txt": fmt.Sprintf("writer %d", index)}})
	}
	errs := make([]error, writers)
	var wg sync.WaitGroup
	for index := range repos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[index] = repos[index].Publish(context.Background(), prepared[index], artifact.PublishOptions{})
		}()
	}
	wg.Wait()
	winners := 0
	for _, err := range errs {
		if err == nil {
			winners++
			continue
		}
		require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	}
	assert.Equal(t, 1, winners)
	assert.Len(t, listNames(t, repos[0]), 1)
	assert.Empty(t, managedKeys(fake, "", layout.ManagedRoot+"/locks"), "every lock was released")
}

// Different key_patterns keep references apart, including a files payload beside an archive.
func TestDistinctLocationsCoexist(t *testing.T) {
	fake := s3test.NewServer(t)
	files := newRepo(t, noneSpec().withPattern("files/{{ .Name }}").with(func(s *rspec) { s.fake = fake }))
	archives := newRepo(t, archiveSpec(artifact.PackagingTgz).withPattern("files/{{ .Name }}.tgz").with(func(s *rspec) { s.fake = fake }))
	publish(t, files, &prepareSpec{name: "one/app"}, artifact.PublishOptions{})
	publish(t, archives, &prepareSpec{name: "two/app"}, artifact.PublishOptions{})
	assertIntact(t, files, ref("one/app", testVersion))
	assertIntact(t, archives, ref("two/app", testVersion))
	assert.Equal(t, []string{"one/app:1.0.0", "two/app:1.0.0"}, listNames(t, files), "both repositories share one namespace")
}
