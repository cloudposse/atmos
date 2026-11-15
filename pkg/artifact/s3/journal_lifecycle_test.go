package s3

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// These tests pin one invariant of the publication journal: every location a journal still owes
// cleanup on stays claimed, survives a replacement and a deletion, and is retired exactly once.
// A journal owes cleanup when a completed publication replaced an earlier one (the Prior) or
// carried the payloads of abandoned attempts, and its retirement did not finish.

// errDeleteDenied is what the gate answers for a delete it was told to refuse.
var errDeleteDenied = errors.New("delete denied")

// gateStore decorates a store so a test can refuse chosen deletes, which is how a retirement fails
// part way: an interrupted retirement is a delete that did not happen.
type gateStore struct {
	blob.Store
	mu     sync.Mutex
	refuse func(key string) bool
}

func (g *gateStore) Delete(ctx context.Context, key string, opts blob.DeleteOptions) error {
	g.mu.Lock()
	denied := g.refuse != nil && g.refuse(key)
	g.mu.Unlock()
	if denied {
		return errDeleteDenied
	}
	return g.Store.Delete(ctx, key, opts)
}

// refuseKeys makes the gate refuse the deletes the predicate selects; nil lets every delete through.
func (g *gateStore) refuseKeys(refuse func(key string) bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.refuse = refuse
}

// shape is one packaging of the payloads a lifecycle test replaces.
type shape struct {
	name      string
	packaging artifact.Packaging
	suffix    string
}

var lifecycleShapes = []shape{
	{name: "files", packaging: artifact.PackagingNone},
	{name: "archive", packaging: artifact.PackagingTgz, suffix: ".tgz"},
}

// lifecycle is one reference replaced once, so its completed journal lists the first publication
// as its Prior. Each generation lives at its own location, as a key_pattern with a date would
// place it, so the locations the journal owes cleanup on are distinct from the live one.
type lifecycle struct {
	t       *testing.T
	shape   shape
	fake    *s3test.Server
	gate    *gateStore
	fault   armedFault
	repos   map[string]*Repository
	base    *artifact.PublishResult
	current *artifact.PublishResult
}

// newLifecycle publishes the first generation under old/. Later generations go through repo.
func newLifecycle(t *testing.T, s shape) *lifecycle {
	t.Helper()
	l := &lifecycle{t: t, shape: s, fake: s3test.NewServer(t), repos: map[string]*Repository{}}
	l.gate = &gateStore{Store: newStoreFor(t, archiveSpec(s.packaging).with(func(c *rspec) { c.fake = l.fake }))}
	l.base = publish(t, l.repo("old"), l.spec("old", "base"), artifact.PublishOptions{})
	return l
}

// repo returns the repository whose key_pattern places the generation under its own directory.
// Every repository shares the bucket and the gate, so a refused delete affects all of them.
func (l *lifecycle) repo(generation string) *Repository {
	l.t.Helper()
	if repo, ok := l.repos[generation]; ok {
		return repo
	}
	spec := archiveSpec(l.shape.packaging).withPattern(generation + "/{{ .Name }}" + l.shape.suffix).with(func(c *rspec) { c.fake = l.fake })
	repo := newRepo(l.t, spec.withOpts(WithStore(l.gate), l.fault.option()))
	l.repos[generation] = repo
	return repo
}

// spec describes one generation's content. The operation ID is stable per generation, so
// publishing it again is the same intent.
func (l *lifecycle) spec(generation, content string) *prepareSpec {
	return &prepareSpec{files: map[string]string{"data.txt": content, generation + ".txt": content}, opID: "op-" + generation}
}

// replace force-publishes a generation over the current one.
func (l *lifecycle) replace(generation string) (*artifact.PublishResult, error) {
	l.t.Helper()
	prior := l.base
	if l.current != nil {
		prior = l.current
	}
	repo := l.repo(generation)
	result, err := repo.Publish(context.Background(), prepare(l.t, repo, l.spec(generation, generation)), artifact.PublishOptions{Force: true, ExpectedPrior: &prior.Pinned})
	if err == nil {
		l.current = result
	}
	return result, err
}

// retirementLeftBehind replaces the first generation while the retirement stops after the replaced
// payload is gone, as a transient delete failure on the Record would.
func (l *lifecycle) retirementLeftBehind() {
	l.t.Helper()
	l.gate.refuseKeys(func(key string) bool { return key == layout.RecordPath(l.base.Pinned.RecordDigest) })
	_, err := l.replace("new")
	require.NoError(l.t, err, "the reference completed; retirement is best effort")
	l.gate.refuseKeys(nil)
	require.Empty(l.t, l.oldPayload(), "the replaced payload was removed before the retirement stopped")
	require.Len(l.t, l.journals(), 1, "the journal keeps the Prior it still owes")
}

// crashedAfterCommit replaces the first generation and stops before any retirement ran.
func (l *lifecycle) crashedAfterCommit() {
	l.t.Helper()
	l.fault.hook = crashAt(faultAfterCommit)
	_, err := l.replace("new")
	require.ErrorIs(l.t, err, errCrash)
	l.fault.hook = nil
	repo := l.repo("new")
	pinned, err := repo.Resolve(context.Background(), defaultRef())
	require.NoError(l.t, err, "a crash after the commit leaves a completed reference")
	l.current = &artifact.PublishResult{Pinned: pinned}
	require.NotEmpty(l.t, l.oldPayload(), "nothing of the replaced payload was retired yet")
}

func (l *lifecycle) journals() []string {
	return managedKeys(l.fake, "", layout.ManagedRoot+"/operations")
}

func (l *lifecycle) records() []string {
	return managedKeys(l.fake, "", layout.ManagedRoot+"/records")
}

// oldPayload lists what is still stored under the first generation's location.
func (l *lifecycle) oldPayload() []string {
	var out []string
	for _, key := range payloadKeys(l.fake, "") {
		if strings.HasPrefix(key, "old/") {
			out = append(out, key)
		}
	}
	return out
}

func (l *lifecycle) payloadUnder(dir string) []string {
	var out []string
	for _, key := range payloadKeys(l.fake, "") {
		if strings.HasPrefix(key, dir+"/") {
			out = append(out, key)
		}
	}
	return out
}

// newcomer is a second reference whose key_pattern resolves onto the first generation's location.
func (l *lifecycle) newcomer() (*Repository, *prepareSpec) {
	l.t.Helper()
	spec := archiveSpec(l.shape.packaging).withPattern("old/team/app" + l.shape.suffix).with(func(c *rspec) { c.fake = l.fake })
	return newRepo(l.t, spec.withOpts(WithStore(l.gate))), &prepareSpec{name: "other/app", files: map[string]string{"a.txt": "newcomer"}}
}

// A retirement that stops after the replaced payload is gone leaves the Prior in the journal while
// its location holds nothing. The location must stay claimed: a newcomer that took it would have
// its objects deleted by the retry, which retires by the journaled keys and cannot tell whose they are.
func TestJournalPrior_LocationStaysClaimedUntilRetired(t *testing.T) {
	for _, s := range lifecycleShapes {
		t.Run(s.name, func(t *testing.T) {
			l := newLifecycle(t, s)
			l.retirementLeftBehind()

			newcomer, newcomerSpec := l.newcomer()
			before := bucketKeys(l.fake)
			_, err := newcomer.Publish(context.Background(), prepare(t, newcomer, newcomerSpec), artifact.PublishOptions{})
			require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
			assert.ErrorContains(t, err, "old/team/app")
			assert.Equal(t, before, bucketKeys(l.fake), "the refused reservation changed nothing")

			// The retry finishes the retirement exactly once and releases the claim.
			retry := publish(t, l.repo("new"), l.spec("new", "new"), artifact.PublishOptions{ExpectedPrior: &l.current.Pinned})
			assert.Equal(t, artifact.OutcomeUnchanged, retry.Outcome)
			assert.Empty(t, l.journals(), "the journal is removed once nothing is owed")
			_, err = l.repo("new").Read(context.Background(), l.base.Pinned)
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "the replaced Record is retired")

			published, err := newcomer.Publish(context.Background(), prepare(t, newcomer, newcomerSpec), artifact.PublishOptions{})
			require.NoError(t, err, "a retired location is claimable again")
			assertIntact(t, newcomer, ref("other/app", testVersion))
			assertIntact(t, l.repo("new"), defaultRef())
			assert.NotEmpty(t, published.Location)
		})
	}
}

// A pending replacement still serves the replaced payload at its old location until the new one
// completes, so that location stays claimed by the reference's journal.
func TestPendingJournal_PriorLocationStaysClaimed(t *testing.T) {
	for _, s := range lifecycleShapes {
		t.Run(s.name, func(t *testing.T) {
			l := newLifecycle(t, s)
			l.fault.hook = crashAt(faultBeforePayload)
			_, err := l.replace("new")
			require.ErrorIs(t, err, errCrash)
			l.fault.hook = nil
			require.NotEmpty(t, l.oldPayload(), "the replaced payload is still stored")

			newcomer, newcomerSpec := l.newcomer()
			_, err = newcomer.Publish(context.Background(), prepare(t, newcomer, newcomerSpec), artifact.PublishOptions{})
			require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
			assert.NotEmpty(t, l.oldPayload(), "the replaced payload was not touched")

			_, err = l.replace("new")
			require.NoError(t, err, "the same-intent retry completes the replacement")
			require.Empty(t, l.oldPayload())
			_, err = newcomer.Publish(context.Background(), prepare(t, newcomer, newcomerSpec), artifact.PublishOptions{})
			require.NoError(t, err)
		})
	}
}

// A forced replacement of a completed publication whose retirement did not finish must not drop what
// that journal still owes: the earlier payload, Record, and the journal itself are retired.
func TestForceReplacement_SettlesTheReplacedJournal(t *testing.T) {
	setups := []struct {
		name  string
		leave func(*lifecycle)
	}{
		{name: "the retirement stopped", leave: (*lifecycle).retirementLeftBehind},
		{name: "the retirement never ran", leave: (*lifecycle).crashedAfterCommit},
	}
	for _, s := range lifecycleShapes {
		for _, setup := range setups {
			t.Run(s.name+"/"+setup.name, func(t *testing.T) {
				l := newLifecycle(t, s)
				setup.leave(l)
				firstGeneration := l.base.Pinned

				_, err := l.replace("third")
				require.NoError(t, err)

				assert.Empty(t, l.oldPayload(), "the payload the first journal owed is gone")
				assert.Empty(t, l.payloadUnder("new"), "the replaced generation's payload is gone")
				assert.NotEmpty(t, l.payloadUnder("third"))
				assert.Empty(t, l.journals(), "no journal is orphaned")
				assert.Len(t, l.records(), 1, "only the live Record remains")
				_, err = l.repo("third").Read(context.Background(), firstGeneration)
				require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
				assertIntact(t, l.repo("third"), defaultRef())
			})
		}
	}
}

// Deleting a completed publication whose retirement did not finish removes everything the journal
// still owed, along with the artifact itself.
func TestDelete_SettlesTheJournalBeforeDeleting(t *testing.T) {
	setups := []struct {
		name  string
		leave func(*lifecycle)
	}{
		{name: "the retirement stopped", leave: (*lifecycle).retirementLeftBehind},
		{name: "the retirement never ran", leave: (*lifecycle).crashedAfterCommit},
	}
	for _, s := range lifecycleShapes {
		for _, setup := range setups {
			t.Run(s.name+"/"+setup.name, func(t *testing.T) {
				l := newLifecycle(t, s)
				setup.leave(l)

				_, err := l.repo("new").Delete(context.Background(), l.current.Pinned, artifact.DeleteOptions{ExpectedCurrent: &l.current.Pinned})
				require.NoError(t, err)

				assert.Empty(t, payloadKeys(l.fake, ""), "no payload object survives")
				assert.Empty(t, l.records(), "no Record survives")
				assert.Empty(t, l.journals(), "no journal survives")
			})
		}
	}
}

// A failure while settling stops the replacement and the deletion, and keeps the journal and its
// claim, so the retry still finds what is owed.
func TestSettlingFailureKeepsTheJournal(t *testing.T) {
	for _, s := range lifecycleShapes {
		t.Run(s.name, func(t *testing.T) {
			l := newLifecycle(t, s)
			l.crashedAfterCommit()
			l.gate.refuseKeys(func(key string) bool { return key == layout.RecordPath(l.base.Pinned.RecordDigest) })

			_, err := l.replace("third")
			require.ErrorIs(t, err, errUtils.ErrArtifactDeleteFailed)
			_, err = l.repo("new").Delete(context.Background(), l.current.Pinned, artifact.DeleteOptions{})
			require.ErrorIs(t, err, errUtils.ErrArtifactDeleteFailed)
			assertIntact(t, l.repo("new"), defaultRef())
			assert.Len(t, l.journals(), 1, "the journal still lists what is owed")

			l.gate.refuseKeys(nil)
			_, err = l.replace("third")
			require.NoError(t, err, "the retry settles and replaces")
			assert.Empty(t, l.journals())
			assert.Len(t, l.records(), 1)
		})
	}
}

// A journal whose Prior is already gone entirely (the Record is deleted last) must still be
// retirable: the retry has nothing left to remove and clears the journal instead of failing forever.
func TestRetirement_IsIdempotentOnceTheRecordIsGone(t *testing.T) {
	for _, s := range lifecycleShapes {
		t.Run(s.name, func(t *testing.T) {
			l := newLifecycle(t, s)
			l.gate.refuseKeys(func(key string) bool { return strings.HasPrefix(key, layout.ManagedRoot+"/operations/") })
			_, err := l.replace("new")
			require.NoError(t, err)
			l.gate.refuseKeys(nil)
			require.Empty(t, l.oldPayload())
			require.Len(t, l.journals(), 1, "only the journal delete failed")
			_, err = l.repo("new").Read(context.Background(), l.base.Pinned)
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "the replaced Record is already retired")

			retry := publish(t, l.repo("new"), l.spec("new", "new"), artifact.PublishOptions{ExpectedPrior: &l.current.Pinned})
			assert.Equal(t, artifact.OutcomeUnchanged, retry.Outcome)
			assert.Empty(t, l.journals())
			assertIntact(t, l.repo("new"), defaultRef())
		})
	}
}

// A newcomer racing the retry that finishes a retirement either is refused while the journal owes
// the location, or takes it once the retirement is done. It is never admitted in between, which
// would let the retry delete the newcomer's objects under the same keys.
func TestJournalPrior_RetirementRacingANewcomerNeverDeletesTheNewcomer(t *testing.T) {
	for _, s := range lifecycleShapes {
		t.Run(s.name, func(t *testing.T) {
			l := newLifecycle(t, s)
			l.retirementLeftBehind()
			newcomer, _ := l.newcomer()
			// The newcomer writes the very keys the journal's Prior lists.
			content := &prepareSpec{name: "other/app", files: map[string]string{"data.txt": "newcomer", "old.txt": "newcomer"}}

			// Preparation fails the test, which only the test goroutine may do.
			retry := prepare(t, l.repo("new"), l.spec("new", "new"))
			incoming := prepare(t, newcomer, content)

			var wg sync.WaitGroup
			errs := make(chan error, 2)
			wg.Add(2)
			go func() {
				defer wg.Done()
				_, err := l.repo("new").Publish(context.Background(), retry, artifact.PublishOptions{ExpectedPrior: &l.current.Pinned})
				errs <- err
			}()
			go func() {
				defer wg.Done()
				var err error
				for range 200 {
					if _, err = newcomer.Publish(context.Background(), incoming, artifact.PublishOptions{}); !errors.Is(err, errUtils.ErrArtifactConflict) {
						break
					}
				}
				errs <- err
			}()
			wg.Wait()
			close(errs)
			for err := range errs {
				require.NoError(t, err)
			}

			assertIntact(t, newcomer, ref("other/app", testVersion))
			assertIntact(t, l.repo("new"), defaultRef())
			assert.Empty(t, l.journals())
		})
	}
}
