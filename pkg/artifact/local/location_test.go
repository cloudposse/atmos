package local

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

const datedPattern = `{{ .CreatedAt | date "2006/01/02" }}/{{ .Name }}/{{ .Version }}`

func datedSpec(kind string, packaging artifact.Packaging) *repoSpec {
	suffix := ""
	if kind == artifact.RepositoryKindLocalArchive {
		suffix = layout.ArchiveExtension(packaging)
	}
	return &repoSpec{kind: kind, packaging: packaging, keyPattern: datedPattern + suffix}
}

var datedSpecs = []*repoSpec{
	datedSpec(artifact.RepositoryKindLocalDir, ""),
	datedSpec(artifact.RepositoryKindLocalArchive, artifact.PackagingTgz),
}

func storedLocation(t *testing.T, repo *Repository, name, version string) string {
	t.Helper()
	tr, err := repo.openTree(false)
	require.NoError(t, err)
	defer tr.Close()
	rep, err := tr.loadRepresentation(layout.ReferenceKey(name, version))
	require.NoError(t, err)
	require.NotNil(t, rep)
	return rep.Location
}

func TestDatedRoot_LocationIsRenderedOnceAndPersisted(t *testing.T) {
	for _, spec := range datedSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			repo := newRepo(t, spec)
			result := publish(t, repo, &prepareSpec{at: time.Date(2026, 10, 30, 23, 59, 59, 0, time.UTC)}, artifact.PublishOptions{})
			want := layout.DefaultLocation(testName, testVersion, repo.packaging)
			want = "2026/10/30/" + want
			assert.Equal(t, filepath.Join(repo.root, filepath.FromSlash(want)), result.Location)
			assert.Equal(t, want, storedLocation(t, repo, testName, testVersion))

			// A new repository object at the same root with a different pattern (and the wall
			// clock long past the capture date) still reads and deletes the persisted location.
			reader := newRepo(t, spec.withRoot(repo.root).withPattern("somewhere/else/{{ .Version }}"))
			assertIntact(t, reader, defaultRef())
			pinned := resolve(t, reader, defaultRef())
			deleted, err := reader.Delete(context.Background(), pinned, artifact.DeleteOptions{Scope: artifact.DeleteObject})
			require.NoError(t, err)
			require.NotEmpty(t, deleted.Deleted)
			assert.Equal(t, result.Location, deleted.Deleted[0], "delete uses the persisted dated location, never a re-rendered one")
			assert.NoDirExists(t, filepath.Join(repo.root, "2026"), "empty dated parents are pruned")
			assert.NoDirExists(t, filepath.Join(repo.root, "somewhere"))
		})
	}
}

func TestDatedRoot_RolloverReusesTheCapturedLocation(t *testing.T) {
	for _, spec := range datedSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			spec = spec.withOpts(withFaults(crashAt(faultAfterMetadata)))
			repo := newRepo(t, spec)
			dayOne := time.Date(2026, 10, 30, 23, 59, 59, 0, time.UTC)
			first := prepare(t, repo, &prepareSpec{at: dayOne, opID: "op-day-one"})
			_, err := repo.Publish(context.Background(), first, artifact.PublishOptions{})
			require.ErrorIs(t, err, errCrash)

			// The same prepared artifact, retried after midnight, lands in the day-one location.
			result, err := repo.Publish(context.Background(), first, artifact.PublishOptions{})
			require.NoError(t, err)
			assert.Contains(t, filepath.ToSlash(result.Location), "2026/10/30/")

			// A later invocation (new preparation time) of the same intent after an interruption
			// also resumes at the journaled location instead of creating a second dated tree.
			repo2 := newRepo(t, spec.withRoot(repo.root).withOpts(withFaults(crashAt(faultAfterMetadata))))
			second := prepare(t, repo2, &prepareSpec{name: "team/other", at: dayOne, opID: "op-other-1"})
			_, err = repo2.Publish(context.Background(), second, artifact.PublishOptions{})
			require.ErrorIs(t, err, errCrash)
			dayTwo := prepare(t, repo2, &prepareSpec{name: "team/other", at: dayOne.Add(26 * time.Hour), opID: "op-other-2"})
			resumed, err := repo2.Publish(context.Background(), dayTwo, artifact.PublishOptions{})
			require.NoError(t, err)
			assert.Contains(t, filepath.ToSlash(resumed.Location), "2026/10/30/", "the journaled location is reused, not re-rendered for the new day")
			assert.NoDirExists(t, filepath.Join(repo.root, "2026", "10", "31"))
			assertIntact(t, repo2, ref("team/other", testVersion))
		})
	}
}

func TestDatedRoot_ForceReplacementAcrossDaysRetiresTheOldLocation(t *testing.T) {
	for _, spec := range datedSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			repo := newRepo(t, spec)
			dayOne := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
			first := publish(t, repo, &prepareSpec{at: dayOne, files: map[string]string{"data.txt": "v1\n"}}, artifact.PublishOptions{})
			old := resolve(t, repo, defaultRef())

			second := publish(t, repo, &prepareSpec{at: dayOne.Add(48 * time.Hour), opID: "op-2", files: map[string]string{"data.txt": "v2\n"}}, artifact.PublishOptions{Force: true, ExpectedPrior: &old})
			assert.Equal(t, artifact.OutcomeReplaced, second.Outcome)
			assert.Contains(t, filepath.ToSlash(second.Location), "2026/11/01/")
			assert.NoFileExists(t, first.Location, "the replaced dated payload is retired")
			assert.NoDirExists(t, filepath.Join(repo.root, "2026", "10"), "its empty dated parents are pruned")
			assertIntact(t, repo, defaultRef())
		})
	}
}

func collidingSpec(kind string) *repoSpec {
	spec := &repoSpec{kind: kind, keyPattern: "shared/{{ .Version }}"}
	if kind == artifact.RepositoryKindLocalArchive {
		spec.packaging = artifact.PackagingTgz
		spec.keyPattern += ".tgz"
	}
	return spec
}

// Two references whose key_pattern resolves to one path: the second fails with a conflict and is
// never bound to the first reference's payload.
func TestCollidingKeyPattern_SecondReferenceConflicts(t *testing.T) {
	for _, kind := range []string{artifact.RepositoryKindLocalDir, artifact.RepositoryKindLocalArchive} {
		t.Run(kind, func(t *testing.T) {
			repo := newRepo(t, collidingSpec(kind))
			first := publish(t, repo, &prepareSpec{name: "a/one", files: map[string]string{"data.txt": "one\n"}}, artifact.PublishOptions{})
			before := resolve(t, repo, ref("a/one", testVersion))

			second := prepare(t, repo, &prepareSpec{name: "b/two", files: map[string]string{"data.txt": "two\n"}})
			_, err := repo.Publish(context.Background(), second, artifact.PublishOptions{Force: true})
			require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
			require.ErrorContains(t, err, "a/one:1.0.0", "the conflict names the owning reference")

			_, err = repo.Resolve(context.Background(), ref("b/two", testVersion))
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "the loser has no reference at all")
			assert.Equal(t, before.RecordDigest, resolve(t, repo, ref("a/one", testVersion)).RecordDigest)
			content, err := repo.Read(context.Background(), before)
			require.NoError(t, err)
			defer content.Close()
			require.NoError(t, readAll(t, content))
			assert.Equal(t, []string{"a/one:1.0.0"}, listNames(t, repo))
			assert.True(t, pathExists(first.Location), "the first payload is untouched")
			assert.Empty(t, listFiles(t, filepath.Join(repo.root, filepath.FromSlash(layout.ManagedRoot), "operations")), "a rejected reservation leaves no journal behind")
		})
	}
}

func TestCollidingKeyPattern_AncestorAndCaseAliases(t *testing.T) {
	spec := &repoSpec{kind: artifact.RepositoryKindLocalDir, keyPattern: "{{ .Kind }}/{{ .Name }}"}
	repo := newRepo(t, spec)
	publish(t, repo, &prepareSpec{name: "tools"}, artifact.PublishOptions{})
	nested := prepare(t, repo, &prepareSpec{name: "tools/cli"})
	_, err := repo.Publish(context.Background(), nested, artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "a payload directory nested in another artifact's directory is a collision")

	caseSpec := &repoSpec{kind: artifact.RepositoryKindLocalDir, keyPattern: "same/{{ .Version }}"}
	caseRepo := newRepo(t, caseSpec)
	publish(t, caseRepo, &prepareSpec{name: "a/x", version: "RC1"}, artifact.PublishOptions{})
	aliased := prepare(t, caseRepo, &prepareSpec{name: "b/y", version: "rc1"})
	_, err = caseRepo.Publish(context.Background(), aliased, artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "locations that differ only by case alias on common filesystems")
}

// Content that exists at the location but is not managed by this repository is never adopted or
// overwritten.
func TestUnmanagedContentAtTheLocationIsNeverOverwritten(t *testing.T) {
	for _, kind := range []string{artifact.RepositoryKindLocalDir, artifact.RepositoryKindLocalArchive} {
		t.Run(kind, func(t *testing.T) {
			spec := &repoSpec{kind: kind}
			if kind == artifact.RepositoryKindLocalArchive {
				spec.packaging = artifact.PackagingTgz
			}
			repo := newRepo(t, spec)
			location := layout.DefaultLocation(testName, testVersion, repo.packaging)
			userFile := filepath.Join(repo.root, filepath.FromSlash(location))
			if kind == artifact.RepositoryKindLocalDir {
				userFile = filepath.Join(userFile, "mine.txt")
			}
			require.NoError(t, os.MkdirAll(filepath.Dir(userFile), 0o755))
			require.NoError(t, os.WriteFile(userFile, []byte("precious"), 0o644))

			prepared := prepare(t, repo, &prepareSpec{})
			_, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
			require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
			require.ErrorContains(t, err, "not managed")
			got, readErr := os.ReadFile(userFile)
			require.NoError(t, readErr)
			assert.Equal(t, "precious", string(got))
			_, err = repo.Resolve(context.Background(), defaultRef())
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
		})
	}
}

// Concurrent publications of colliding references: exactly one wins.
func TestCollidingKeyPattern_ConcurrentWritersExactlyOneWins(t *testing.T) {
	repo := newRepo(t, collidingSpec(artifact.RepositoryKindLocalDir))
	names := []string{"a/one", "b/two", "c/three", "d/four"}
	prepared := make([]*artifact.PreparedArtifact, len(names))
	for index, name := range names {
		prepared[index] = prepare(t, repo, &prepareSpec{name: name, files: map[string]string{"data.txt": name}})
	}
	errs := make([]error, len(names))
	var wg sync.WaitGroup
	for index := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[index] = repo.Publish(context.Background(), prepared[index], artifact.PublishOptions{})
		}()
	}
	wg.Wait()
	winners := 0
	for _, err := range errs {
		if err == nil {
			winners++
		} else {
			require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
		}
	}
	assert.Equal(t, 1, winners, "ownership of the shared location is exclusive")
	assert.Len(t, listNames(t, repo), 1)
	for _, name := range listNames(t, repo) {
		parsed, err := artifact.ParseRef(name)
		require.NoError(t, err)
		assertIntact(t, repo, parsed)
	}
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
