package oci

import (
	"context"
	"sync"
	"testing"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// rendezvous lets the first two readers of the destination tag meet, so a test can make two
// concurrent copies read the tag before either writes it. A reader that cannot meet its peer
// within the window (because the peer is blocked behind a lock) carries on alone.
type rendezvous struct {
	mu      sync.Mutex
	arrived int
	met     chan struct{}
	window  time.Duration
}

func newRendezvous(window time.Duration) *rendezvous {
	return &rendezvous{met: make(chan struct{}), window: window}
}

func (r *rendezvous) arrive() {
	r.mu.Lock()
	r.arrived++
	if r.arrived == 2 {
		close(r.met)
	}
	r.mu.Unlock()
	select {
	case <-r.met:
	case <-time.After(r.window):
	}
}

// rendezvousBackend opens write stores whose tag reads meet at the rendezvous.
type rendezvousBackend struct {
	backend
	tag   string
	point *rendezvous
}

func (b *rendezvousBackend) open(ctx context.Context, name string, write bool) (store, error) {
	opened, err := b.backend.open(ctx, name, write)
	if err != nil || !write {
		return opened, err
	}
	return &rendezvousStore{store: opened, tag: b.tag, point: b.point}, nil
}

type rendezvousStore struct {
	store
	tag   string
	point *rendezvous
}

func (s *rendezvousStore) Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error) {
	desc, err := s.store.Resolve(ctx, reference)
	if reference == s.tag {
		s.point.arrive()
	}
	return desc, err
}

// Two copies of different content into one registry tag, with no caller-held lock, must not both
// pass the "tag is absent" decision: the copy takes the reference lock itself, so the second one
// reads the tag the first wrote and loses with a conflict instead of silently overwriting it.
func TestCopyFrom_RegistryDestinationSerializesCallersWithoutALock(t *testing.T) {
	ctx := context.Background()
	registry := newRegistry(t, nil)
	destination := newRegistryRepo(t, registry)
	destination.be = &rendezvousBackend{backend: destination.be, tag: destination.be.reference(appName, appVersion), point: newRendezvous(500 * time.Millisecond)}

	sources := make([]*Repository, 2)
	pins := make([]artifact.PinnedReference, 2)
	for index, content := range []string{"first writer", "second writer"} {
		sources[index] = newLayoutRepo(t)
		pins[index] = publish(t, sources[index], publishSpec{content: content}).Pinned
	}
	require.NotEqual(t, pins[0].ResolvedDigest, pins[1].ResolvedDigest)

	errs := make([]error, len(sources))
	results := make([]*artifact.PublishResult, len(sources))
	var wg sync.WaitGroup
	for index := range sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[index], errs[index] = destination.CopyFrom(ctx, sources[index], pins[index], artifact.CopyFromOptions{})
		}()
	}
	wg.Wait()

	winner := -1
	for index, err := range errs {
		if err == nil {
			require.Equal(t, -1, winner, "only one copy may create the tag")
			winner = index
			continue
		}
		require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	}
	require.NotEqual(t, -1, winner, "one copy creates the tag")
	assert.Equal(t, artifact.OutcomeCreated, results[winner].Outcome)
	current, err := destination.Resolve(ctx, artifact.Ref{Name: appName, Version: appVersion})
	require.NoError(t, err)
	assert.Equal(t, pins[winner].ResolvedDigest, current.ResolvedDigest, "the tag still names the winner's content")
}

// A caller that already holds the writer lock keeps it: the copy never waits on a second claim for
// the same reference, which would deadlock behind the caller's own guard.
func TestCopyFrom_RegistryDestinationUsesTheCallersLock(t *testing.T) {
	registry := newRegistry(t, nil)
	destination := newRegistryRepo(t, registry)
	source := newLayoutRepo(t)
	pinned := publish(t, source, publishSpec{content: "held lock"}).Pinned

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	held, err := destination.Lock(ctx, artifact.LockRequest{Resource: artifact.Ref{Name: appName, Version: appVersion}.String()})
	require.NoError(t, err)
	result, err := destination.CopyFrom(ctx, source, pinned, artifact.CopyFromOptions{Lock: held})
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeCreated, result.Outcome)
	assert.Equal(t, artifact.LockScopeHost, result.LockScope)
	require.NoError(t, held.Release(ctx), "the copy left the caller's lock held")
}
