package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

var allSpecs = []*repoSpec{dirSpec(), archiveSpec(artifact.PackagingTgz), archiveSpec(artifact.PackagingZip)}

func specName(spec *repoSpec) string { return spec.kind + "/" + string(spec.packaging) }

func listNames(t *testing.T, repo *Repository) []string {
	t.Helper()
	page, err := repo.List(context.Background(), artifact.ListQuery{})
	require.NoError(t, err)
	names := make([]string, 0, len(page.Entries))
	for index := range page.Entries {
		names = append(names, page.Entries[index].Name+":"+page.Entries[index].Version)
	}
	return names
}

// An interruption before the commit never advertises partial success: the reference is
// unavailable (not found, not complete-looking), invisible to listing, and a same-intent retry
// repairs it completely.
func TestInterruptedPublication_RetryRepairs(t *testing.T) {
	points := []faultPoint{faultBeforePayload, faultDuringPayload, faultAfterMetadata}
	for _, spec := range allSpecs {
		for _, point := range points {
			t.Run(specName(spec)+"/"+string(point), func(t *testing.T) {
				spec = spec.withOpts(withFaults(crashAt(point)))
				repo := newRepo(t, spec)
				prepared := prepare(t, repo, &prepareSpec{})

				_, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
				require.ErrorIs(t, err, errCrash)

				_, err = repo.Resolve(context.Background(), defaultRef())
				require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable, "a pending reference is unavailable to managed readers")
				require.ErrorContains(t, err, prepared.OperationID, "the diagnostic names the owning operation")
				assert.Empty(t, listNames(t, repo), "a pending reference is absent from search results")
				_, err = repo.Pin(context.Background(), defaultRef())
				require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable)
				recordDigest, err := prepared.Record.RecordDigest()
				require.NoError(t, err)
				_, err = repo.Resolve(context.Background(), artifact.Ref{Name: testName, Digest: recordDigest})
				require.Error(t, err, "even the digest does not expose a pending record")
				assert.True(t, errors.Is(err, errUtils.ErrArtifactPublicationUnavailable) || errors.Is(err, errUtils.ErrArtifactNotFound), "%v", err)

				retry, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
				require.NoError(t, err)
				assert.Equal(t, artifact.OutcomeCreated, retry.Outcome)
				assertIntact(t, repo, defaultRef())
				assert.Equal(t, []string{testName + ":" + testVersion}, listNames(t, repo))
				assert.NoDirExists(t, filepath.Join(repo.root, filepath.FromSlash(layout.StagingPath(prepared.OperationID))), "the retry cleans the operation's staging")
				assert.NoFileExists(t, filepath.Join(repo.root, filepath.FromSlash(layout.OperationPath(prepared.OperationID))))
			})
		}
	}
}

// assertIntact reads the artifact end to end and checks it verifies.
func assertIntact(t *testing.T, repo *Repository, r artifact.Ref) {
	t.Helper()
	pinned := resolve(t, repo, r)
	content, err := repo.Read(context.Background(), pinned)
	require.NoError(t, err)
	defer content.Close()
	require.NoError(t, readAll(t, content))
}

// A crash after the commit but before the response leaves a completed reference: a retry is a
// no-op, and the caller can prove success by reading it.
func TestInterruptedPublication_AfterCommitIsComplete(t *testing.T) {
	for _, spec := range allSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			spec = spec.withOpts(withFaults(crashAt(faultAfterCommit)))
			repo := newRepo(t, spec)
			prepared := prepare(t, repo, &prepareSpec{})

			_, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
			require.ErrorIs(t, err, errCrash)

			assertIntact(t, repo, defaultRef())
			digestWant, err := prepared.Record.RecordDigest()
			require.NoError(t, err)
			assert.Equal(t, digestWant, resolve(t, repo, defaultRef()).RecordDigest)

			_, err = repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
			require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "a writer that expects the reference to be absent is stale")
			current := resolve(t, repo, defaultRef())
			retry, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{ExpectedPrior: &current})
			require.NoError(t, err)
			assert.Equal(t, artifact.OutcomeUnchanged, retry.Outcome)
			assert.NoDirExists(t, filepath.Join(repo.root, filepath.FromSlash(layout.StagingPath(prepared.OperationID))), "an unchanged republish tidies the finished operation")
		})
	}
}

// A different intent never steals a pending operation, with or without force. The same intent
// from a later invocation (new operation ID and preparation time) repairs it.
func TestPendingOperation_IsNeverStolen(t *testing.T) {
	for _, spec := range allSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			spec = spec.withOpts(withFaults(crashAt(faultAfterMetadata)))
			repo := newRepo(t, spec)
			first := prepare(t, repo, &prepareSpec{files: map[string]string{"data.txt": "first\n"}, opID: "op-first"})
			_, err := repo.Publish(context.Background(), first, artifact.PublishOptions{})
			require.ErrorIs(t, err, errCrash)

			other := prepare(t, repo, &prepareSpec{files: map[string]string{"data.txt": "different\n"}, opID: "op-other"})
			for _, force := range []bool{false, true} {
				_, err := repo.Publish(context.Background(), other, artifact.PublishOptions{Force: force})
				require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable, "force=%v", force)
				require.ErrorContains(t, err, "op-first", "the recovery diagnostic includes the operation ID")
			}
			_, err = repo.Resolve(context.Background(), defaultRef())
			require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable, "the failed attempt did not disturb the pending state")

			later := prepare(t, repo, &prepareSpec{files: map[string]string{"data.txt": "first\n"}, opID: "op-later", at: capturedAt.Add(72 * time.Hour)})
			result, err := repo.Publish(context.Background(), later, artifact.PublishOptions{})
			require.NoError(t, err, "the same intent from a later invocation repairs the pending operation")
			assert.Equal(t, artifact.OutcomeCreated, result.Outcome)
			assertIntact(t, repo, defaultRef())
			assert.NoFileExists(t, filepath.Join(repo.root, filepath.FromSlash(layout.OperationPath("op-first"))), "the abandoned journal is retired")
		})
	}
}

// A failed or interrupted force replacement leaves the reference unavailable until the same-intent
// retry repairs it. The old digest never serves the replacement's bytes and is not found after.
func TestInterruptedForceReplacement(t *testing.T) {
	for _, spec := range allSpecs {
		for _, point := range []faultPoint{faultDuringPayload, faultAfterMetadata} {
			t.Run(specName(spec)+"/"+string(point), func(t *testing.T) {
				var armed faultHook
				spec.opts = []Option{withFaults(func(p faultPoint) error {
					if armed != nil {
						return armed(p)
					}
					return nil
				})}
				repo := newRepo(t, spec)
				publish(t, repo, &prepareSpec{files: map[string]string{"data.txt": "v1\n"}, opID: "op-1"}, artifact.PublishOptions{})
				old := resolve(t, repo, defaultRef())

				replacement := prepare(t, repo, &prepareSpec{files: map[string]string{"data.txt": "v2\n"}, opID: "op-2"})
				armed = crashAt(point)
				_, err := repo.Publish(context.Background(), replacement, artifact.PublishOptions{Force: true, ExpectedPrior: &old})
				require.ErrorIs(t, err, errCrash)

				_, err = repo.Resolve(context.Background(), defaultRef())
				require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable, "the interrupted replacement leaves the reference unavailable")
				_, err = repo.Read(context.Background(), old)
				require.Error(t, err, "the old digest is never served while the replacement is in flight")
				assert.Empty(t, listNames(t, repo))

				result, err := repo.Publish(context.Background(), replacement, artifact.PublishOptions{Force: true})
				require.NoError(t, err, "the same-intent retry repairs the reference")
				assert.Equal(t, artifact.OutcomeReplaced, result.Outcome)
				current := resolve(t, repo, defaultRef())
				assert.NotEqual(t, old.RecordDigest, current.RecordDigest)
				assertIntact(t, repo, defaultRef())

				_, err = repo.Read(context.Background(), old)
				require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "retired bytes report not found, never the current bytes")
				_, err = repo.Resolve(context.Background(), artifact.Ref{Name: testName, Digest: old.RecordDigest})
				require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
			})
		}
	}
}

// Old-digest reads never return the replacement's bytes, even when the physical location is the
// same, and the guard catches a reader that is mid-stream when the replacement lands.
func TestReplacementNeverServesNewBytesUnderOldDigest(t *testing.T) {
	for _, spec := range allSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			repo := newRepo(t, spec)
			publish(t, repo, &prepareSpec{files: map[string]string{"data.txt": "v1\n"}}, artifact.PublishOptions{})
			old := resolve(t, repo, defaultRef())
			content, err := repo.Read(context.Background(), old)
			require.NoError(t, err)
			defer content.Close()

			publish(t, repo, &prepareSpec{files: map[string]string{"data.txt": "v2\n"}, opID: "op-2"}, artifact.PublishOptions{Force: true, ExpectedPrior: &old})

			err = readAll(t, content)
			require.Error(t, err, "a reader that was open when the reference moved must not report success")
			assert.True(t, errorsIsAny(err, errUtils.ErrArtifactPublicationUnavailable, errUtils.ErrArtifactIntegrityFailed, errUtils.ErrArtifactNotFound), "%v", err)

			_, err = repo.Read(context.Background(), old)
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
		})
	}
}

// The recovery path triggers only for the matching condition: without an interruption a changed
// intent is a plain conflict and nothing is repaired or replaced.
func TestNoRecoveryWithoutInterruption(t *testing.T) {
	repo := newRepo(t, dirSpec())
	publish(t, repo, &prepareSpec{files: map[string]string{"data.txt": "v1\n"}}, artifact.PublishOptions{})
	old := resolve(t, repo, defaultRef())

	changed := prepare(t, repo, &prepareSpec{files: map[string]string{"data.txt": "v2\n"}, opID: "op-2"})
	_, err := repo.Publish(context.Background(), changed, artifact.PublishOptions{ExpectedPrior: &old})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	assert.Equal(t, old.RecordDigest, resolve(t, repo, defaultRef()).RecordDigest, "the stored artifact is untouched")
	assertIntact(t, repo, defaultRef())

	stale := artifact.PinnedReference{ResolvedDigest: digest.FromString("stale")}
	_, err = repo.Publish(context.Background(), changed, artifact.PublishOptions{Force: true, ExpectedPrior: &stale})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "a stale expected prior is a conflict even under force")

	_, err = repo.Publish(context.Background(), changed, artifact.PublishOptions{Force: true})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "the reference exists but the writer expected none")
}

// Identical republication is unchanged even when time and operation differ.
func TestRepublishIsUnchanged(t *testing.T) {
	for _, spec := range allSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			repo := newRepo(t, spec)
			first := publish(t, repo, &prepareSpec{}, artifact.PublishOptions{})
			before := resolve(t, repo, defaultRef())
			pinned := before
			second := publish(t, repo, &prepareSpec{at: capturedAt.Add(48 * time.Hour), opID: "op-again"}, artifact.PublishOptions{ExpectedPrior: &pinned})
			assert.Equal(t, artifact.OutcomeUnchanged, second.Outcome)
			assert.Equal(t, first.Pinned.RecordDigest, second.Pinned.RecordDigest, "the existing Record is returned unchanged")
			assert.Equal(t, first.Location, second.Location)
			assert.Equal(t, before.RecordDigest, resolve(t, repo, defaultRef()).RecordDigest)
		})
	}
}

func errorsIsAny(err error, targets ...error) bool {
	for _, target := range targets {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// A failed publication before any state change (validation, unsupported packaging) writes nothing.
func TestRejectedPublicationWritesNothing(t *testing.T) {
	repo := newRepo(t, dirSpec())
	wrong := prepare(t, newRepo(t, archiveSpec(artifact.PackagingTgz)), &prepareSpec{})
	_, err := repo.Publish(context.Background(), wrong, artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "a dir repository does not accept an archive payload")
	_, statErr := os.Stat(repo.root)
	assert.True(t, os.IsNotExist(statErr), "the root was not even created")

	_, err = repo.Publish(context.Background(), nil, artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
}

// A same-intent retry under a different packaging cannot reuse the journaled location (its
// extension would disagree), so it publishes at a freshly rendered location and retires the
// abandoned attempt's partial payload instead of orphaning it.
func TestResumedPublicationRetiresAnAbandonedLocation(t *testing.T) {
	var fault armedFault
	root := filepath.Join(t.TempDir(), "repo")
	tgz := archiveSpec(artifact.PackagingTgz).withRoot(root).withOpts(fault.option())
	repo := newRepo(t, tgz)
	fault.hook = crashAt(faultAfterMetadata)
	_, err := repo.Publish(context.Background(), prepare(t, repo, &prepareSpec{opID: "op-tgz"}), artifact.PublishOptions{})
	require.ErrorIs(t, err, errCrash)
	abandoned := filepath.Join(root, "team", "app", "1.0.0.tgz")
	assert.FileExists(t, abandoned, "the crashed attempt left its payload behind")

	zipRepo := newRepo(t, archiveSpec(artifact.PackagingZip).withRoot(root))
	result, err := zipRepo.Publish(context.Background(), prepare(t, zipRepo, &prepareSpec{opID: "op-zip"}), artifact.PublishOptions{})
	require.NoError(t, err, "the same intent repairs the pending reference whatever the packaging")
	assert.Equal(t, filepath.Join(root, "team", "app", "1.0.0.zip"), result.Location)
	assert.NoFileExists(t, abandoned, "the abandoned payload was retired, not orphaned")
	assert.NoFileExists(t, abandoned+".metadata.json")
	assertIntact(t, zipRepo, defaultRef())
	assert.Equal(t, []string{"team/app/1.0.0.zip", "team/app/1.0.0.zip.metadata.json"}, payloadFiles(t, root))
}
