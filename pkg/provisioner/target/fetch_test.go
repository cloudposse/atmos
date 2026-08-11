package target

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// fakeFetcher implements both Provisioner and the optional Fetcher capability.
type fakeFetcher struct {
	artifact ProvisionArtifact
	fetched  int
}

func (f *fakeFetcher) Deliver(context.Context, *DeliverInput) error { return nil }

func (f *fakeFetcher) Fetch(_ context.Context, _ *FetchInput) (ProvisionArtifact, error) {
	f.fetched++
	return f.artifact, nil
}

func TestFetchRoutesToFetcher(t *testing.T) {
	const kind = "test-kind-fetch"
	inst := &fakeFetcher{artifact: ProvisionArtifact{Files: map[string][]byte{"a.yaml": []byte("kind: A")}}}
	Register(kind, inst)

	got, err := Fetch(context.Background(), kind, &FetchInput{TargetName: "t"})
	require.NoError(t, err)
	assert.Equal(t, 1, inst.fetched)
	require.Contains(t, got.Files, "a.yaml")
	assert.Equal(t, []byte("kind: A"), got.Files["a.yaml"])
}

func TestFetchUnknownKind(t *testing.T) {
	_, err := Fetch(context.Background(), "nope-not-registered", &FetchInput{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProvisionTargetKindUnknown)
}

func TestFetchNoFetchCapability(t *testing.T) {
	const kind = "test-kind-no-fetch"
	// fakeProvisioner (from target_test.go) implements Deliver only, not Fetcher.
	Register(kind, &fakeProvisioner{})

	_, err := Fetch(context.Background(), kind, &FetchInput{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProvisionTargetNoFetch)
}
