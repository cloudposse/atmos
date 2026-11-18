package s3

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// These tests pin the recovery of a completed reference whose representation no longer names its
// payload. The journal that wrote the payload is then the only record of where the payload lives, so
// a replacement may not remove that journal before the location is carried into the new journal.

// representationKey is the stored key of the lifecycle reference's representation.
func representationKey() string {
	return layout.RepresentationPath(layout.ReferenceKey(testName, testVersion))
}

// rawRepresentation returns the stored representation bytes of the lifecycle reference.
func (l *lifecycle) rawRepresentation(repo *Repository) []byte {
	l.t.Helper()
	raw, _, err := repo.readObject(context.Background(), representationKey(), layout.MaxDocumentBytes)
	require.NoError(l.t, err)
	require.NotEmpty(l.t, raw)
	return raw
}

// orphanedRepresentation is how a completed reference loses the record of its payload location.
type orphanedRepresentation struct {
	name  string
	apply func(l *lifecycle, staleRepresentation []byte)
}

var orphanings = []orphanedRepresentation{
	{name: "the representation is missing", apply: func(l *lifecycle, _ []byte) {
		require.NoError(l.t, l.repo("new").deleteObject(context.Background(), representationKey()))
	}},
	{name: "the representation describes the replaced Record", apply: func(l *lifecycle, stale []byte) {
		l.fake.Seed(representationKey(), stale, nil)
	}},
}

// completedWithOrphanedRepresentation leaves a completed reference at the "new" location whose
// journal still owes the replaced payload and whose representation does not name the live payload.
func completedWithOrphanedRepresentation(t *testing.T, s shape, orphaning orphanedRepresentation) *lifecycle {
	t.Helper()
	l := newLifecycle(t, s)
	stale := l.rawRepresentation(l.repo("old"))
	l.crashedAfterCommit()
	orphaning.apply(l, stale)
	require.Len(t, l.journals(), 1, "the journal is the only record of the live location")
	require.NotEmpty(t, l.payloadUnder("new"))
	return l
}

// currentReference returns the stored reference of the lifecycle artifact.
func (l *lifecycle) currentReference() *layout.Reference {
	l.t.Helper()
	state, err := l.repo("new").loadReference(context.Background(), layout.ReferenceKey(testName, testVersion))
	require.NoError(l.t, err)
	require.NotNil(l.t, state)
	return state.ref
}

// journalID is the operation the completed reference points at.
func (l *lifecycle) journalID() string { return l.currentReference().OperationID }

// pendingJournal returns the journal of the pending reference, read through the generation's repository.
func (l *lifecycle) pendingJournal(generation string) *layout.Operation {
	l.t.Helper()
	state, err := l.repo(generation).loadReference(context.Background(), layout.ReferenceKey(testName, testVersion))
	require.NoError(l.t, err)
	require.NotNil(l.t, state)
	require.False(l.t, state.ref.Available(), "the reference is pending")
	journal, err := l.repo(generation).loadOperation(context.Background(), state.ref.OperationID)
	require.NoError(l.t, err)
	require.NotNil(l.t, journal)
	return journal
}

// newcomerAt is a second reference whose key_pattern resolves onto the given location.
func (l *lifecycle) newcomerAt(location string) (*Repository, *prepareSpec) {
	l.t.Helper()
	spec := archiveSpec(l.shape.packaging).withPattern(location + l.shape.suffix).with(func(c *rspec) { c.fake = l.fake })
	return newRepo(l.t, spec.withOpts(WithStore(l.gate))), &prepareSpec{name: "other/app", files: map[string]string{"a.txt": "newcomer"}}
}

// repair force-publishes different content for the generation over the current reference.
func (l *lifecycle) repair(generation string) (*artifact.PublishResult, error) {
	l.t.Helper()
	repo := l.repo(generation)
	spec := &prepareSpec{files: map[string]string{"repaired.txt": "repaired-" + generation}, opID: "op-repair-" + generation}
	return repo.Publish(context.Background(), prepare(l.t, repo, spec), artifact.PublishOptions{Force: true, ExpectedPrior: &l.current.Pinned})
}

// A forced repair of a reference whose representation does not name its payload settles the journal
// without losing the live location: the repair at the same location owns its payload, and a repair
// at another location retires the payload that was left behind.
func TestForceRepair_KeepsTheRecoveredPayloadLocation(t *testing.T) {
	targets := []struct {
		name       string
		generation string
		wantAt     string
	}{
		{name: "at the same location", generation: "new", wantAt: "new"},
		{name: "at another location", generation: "third", wantAt: "third"},
	}
	for _, s := range lifecycleShapes {
		for _, orphaning := range orphanings {
			for _, target := range targets {
				t.Run(s.name+"/"+orphaning.name+"/"+target.name, func(t *testing.T) {
					l := completedWithOrphanedRepresentation(t, s, orphaning)

					_, err := l.repair(target.generation)
					require.NoError(t, err, "the live payload is owned by the reference being repaired")

					assert.Empty(t, l.oldPayload(), "the payload the journal owed is retired")
					assert.NotEmpty(t, l.payloadUnder(target.wantAt))
					if target.generation != "new" {
						assert.Empty(t, l.payloadUnder("new"), "the payload the journal located is retired, not orphaned")
					}
					assert.Empty(t, l.journals(), "no journal is orphaned")
					assert.Len(t, l.records(), 1, "only the live Record remains")
					assertIntact(t, l.repo(target.generation), defaultRef())
				})
			}
		}
	}
}

// A completed reference whose journal owes nothing still has that journal as the only record of
// its payload when the representation is missing, so the repair keeps it until the new journal
// names the location.
func TestForceRepair_KeepsAJournalThatOwesNothing(t *testing.T) {
	for _, s := range lifecycleShapes {
		t.Run(s.name, func(t *testing.T) {
			l := completedWithOrphanedRepresentation(t, s, orphanings[0])
			repo := l.repo("new")
			journal, err := repo.loadOperation(context.Background(), l.journalID())
			require.NoError(t, err)
			journal.Prior, journal.Abandoned = nil, nil
			require.NoError(t, repo.writeOperation(context.Background(), journal))
			require.False(t, owes(journal))

			recovered, err := repo.settleReplaced(context.Background(), l.currentReference())
			require.NoError(t, err)
			require.NotNil(t, recovered, "the journal locates the payload")
			assert.Equal(t, journal.Location, recovered.Location)
			assert.Len(t, l.journals(), 1, "the journal is not removed while it is the only record of the location")

			_, err = l.repair("third")
			require.NoError(t, err)
			assert.Empty(t, l.payloadUnder("new"), "the located payload is retired")
			assert.Empty(t, l.journals())
			assertIntact(t, l.repo("third"), defaultRef())
		})
	}
}

// settleReplaced hands back what it recovered and keeps the journal that is the only record of it.
// A representation that names the live payload needs no recovery, and its journal is removed.
func TestSettleReplaced_ReturnsTheRecoveredRepresentation(t *testing.T) {
	for _, s := range lifecycleShapes {
		t.Run(s.name+"/recovered", func(t *testing.T) {
			l := completedWithOrphanedRepresentation(t, s, orphanings[0])
			repo := l.repo("new")
			journal, err := repo.loadOperation(context.Background(), l.journalID())
			require.NoError(t, err)

			recovered, err := repo.settleReplaced(context.Background(), l.currentReference())
			require.NoError(t, err)
			require.NotNil(t, recovered)
			assert.Equal(t, journal.Location, recovered.Location)
			assert.Equal(t, journal.Packaging, recovered.Packaging)
			assert.Equal(t, journal.RecordDigest, recovered.RecordDigest)
			assert.Equal(t, journal.Payload, recovered.Payload)
			assert.Equal(t, journal.Documents, recovered.Documents)
			assert.Empty(t, l.oldPayload(), "what the journal owed is retired")
			assert.Len(t, l.journals(), 1, "the journal still locates the live payload")
		})
		t.Run(s.name+"/intact", func(t *testing.T) {
			l := newLifecycle(t, s)
			l.crashedAfterCommit()

			recovered, err := l.repo("new").settleReplaced(context.Background(), l.currentReference())
			require.NoError(t, err)
			assert.Nil(t, recovered, "the stored representation already names the payload")
			assert.Empty(t, l.journals(), "the journal is removed once settled")
		})
	}
}

// A crash between the settlement and the new journal leaves the reference as it was, so the retry
// finds the location again.
func TestForceRepair_SurvivesACrashAfterSettling(t *testing.T) {
	for _, s := range lifecycleShapes {
		for _, orphaning := range orphanings {
			t.Run(s.name+"/"+orphaning.name, func(t *testing.T) {
				l := completedWithOrphanedRepresentation(t, s, orphaning)
				_, err := l.repo("new").settleReplaced(context.Background(), l.currentReference())
				require.NoError(t, err)
				require.Len(t, l.journals(), 1)

				_, err = l.repair("third")
				require.NoError(t, err)
				assert.Empty(t, l.payloadUnder("new"))
				assert.Empty(t, l.journals())
				assertIntact(t, l.repo("third"), defaultRef())
			})
		}
	}
}

// A crash once the repair is pending leaves the old location in the new journal's Prior: it stays
// claimed, and the same-intent retry completes the repair and retires it.
func TestForceRepair_SurvivesACrashAfterTheNewJournal(t *testing.T) {
	for _, s := range lifecycleShapes {
		for _, orphaning := range orphanings {
			t.Run(s.name+"/"+orphaning.name, func(t *testing.T) {
				l := completedWithOrphanedRepresentation(t, s, orphaning)
				l.fault.hook = crashAt(faultBeforePayload)
				_, err := l.repair("third")
				require.ErrorIs(t, err, errCrash)
				l.fault.hook = nil

				assert.NotEmpty(t, l.payloadUnder("new"), "the live payload is untouched while the repair is pending")
				assert.Len(t, l.journals(), 1, "the pending journal replaced the settled one")

				_, err = l.repair("third")
				require.NoError(t, err, "the same-intent retry completes the repair")
				assert.Empty(t, l.payloadUnder("new"), "the located payload is retired")
				assert.Empty(t, l.journals())
				assertIntact(t, l.repo("third"), defaultRef())
			})
		}
	}
}

// The recovered location is claimed by the pending repair, so no other reference reserves it.
func TestForceRepair_PendingRepairKeepsTheLocationClaimed(t *testing.T) {
	for _, s := range lifecycleShapes {
		t.Run(s.name, func(t *testing.T) {
			l := completedWithOrphanedRepresentation(t, s, orphanings[0])
			l.fault.hook = crashAt(faultBeforePayload)
			_, err := l.repair("third")
			require.ErrorIs(t, err, errCrash)
			l.fault.hook = nil

			journal := l.pendingJournal("third")
			require.NotNil(t, journal.Prior)
			assert.NotEmpty(t, journal.Prior.Location, "the new journal names the located payload")

			// A newcomer whose key_pattern resolves onto the located payload is refused.
			newcomer, spec := l.newcomerAt("new/team/app")
			before := bucketKeys(l.fake)
			_, err = newcomer.Publish(context.Background(), prepare(t, newcomer, spec), artifact.PublishOptions{})
			require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
			assert.Equal(t, before, bucketKeys(l.fake), "the refused reservation changed nothing")
		})
	}
}

// Negative path: nothing recovers a payload whose location no record names. The repair does not
// invent one, and it does not fail the replacement that cannot reach the unknown payload.
func TestSettleReplaced_NoRecoveryWithoutAJournalForTheRecord(t *testing.T) {
	for _, s := range lifecycleShapes {
		t.Run(s.name, func(t *testing.T) {
			l := completedWithOrphanedRepresentation(t, s, orphanings[0])
			repo := l.repo("new")
			journal, err := repo.loadOperation(context.Background(), l.journalID())
			require.NoError(t, err)
			journal.RecordDigest = l.base.Pinned.RecordDigest
			require.NoError(t, repo.writeOperation(context.Background(), journal))

			_, err = repo.settleReplaced(context.Background(), l.currentReference())
			require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
			assert.Len(t, l.journals(), 1, "a failed settlement keeps the journal")
		})
	}
}

// A completed reference whose representation does not name its payload still claims the location the
// journal records: a newcomer that took it would have its objects deleted by the retirement or the
// repair, which act by the journaled keys.
func TestClaim_LocationRecordedOnlyByTheJournalStaysClaimed(t *testing.T) {
	for _, s := range lifecycleShapes {
		for _, orphaning := range orphanings {
			t.Run(s.name+"/"+orphaning.name, func(t *testing.T) {
				l := completedWithOrphanedRepresentation(t, s, orphaning)
				// With the payload objects gone, the claim is the only thing keeping the location.
				for _, key := range l.payloadUnder("new") {
					require.NoError(t, l.gate.Store.Delete(context.Background(), key, blob.DeleteOptions{}))
				}
				require.Empty(t, l.payloadUnder("new"))
				newcomer, spec := l.newcomerAt("new/team/app")
				before := bucketKeys(l.fake)

				_, err := newcomer.Publish(context.Background(), prepare(t, newcomer, spec), artifact.PublishOptions{})

				require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
				assert.Equal(t, before, bucketKeys(l.fake), "the refused reservation changed nothing")
			})
		}
	}
}
