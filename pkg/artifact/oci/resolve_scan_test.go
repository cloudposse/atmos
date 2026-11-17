//nolint:gocritic // OCI descriptors are passed by value, mirroring the ORAS content contracts.
package oci

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// unavailableBackend opens stores whose blob reads fail like a registry that answers 503, while
// tag resolution keeps working.
type unavailableBackend struct {
	backend
}

func (b *unavailableBackend) open(ctx context.Context, name string, write bool) (store, error) {
	opened, err := b.backend.open(ctx, name, write)
	if err != nil {
		return nil, err
	}
	return &unavailableStore{store: opened}, nil
}

type unavailableStore struct {
	store
}

var errServiceUnavailable = errors.New("503 service unavailable")

func (s *unavailableStore) Fetch(context.Context, ocispec.Descriptor) (io.ReadCloser, error) {
	return nil, errServiceUnavailable
}

// tamperedBackend serves wrong bytes for one manifest digest, like a registry returning content
// that fails digest verification. Everything else reads normally.
type tamperedBackend struct {
	backend
	target digest.Digest
}

func (b *tamperedBackend) open(ctx context.Context, name string, write bool) (store, error) {
	opened, err := b.backend.open(ctx, name, write)
	if err != nil {
		return nil, err
	}
	return &tamperedStore{store: opened, target: b.target}, nil
}

type tamperedStore struct {
	store
	target digest.Digest
}

func (s *tamperedStore) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	if desc.Digest == s.target {
		return io.NopCloser(bytes.NewReader([]byte("tampered"))), nil
	}
	return s.store.Fetch(ctx, desc)
}

// A Record-digest lookup scans the tags of a name. A tag that resolves but whose manifest cannot be
// fetched because the registry is failing is an operational failure, not proof that the digest is
// absent; a root that is merely unusable stays a nonmatch.
func TestScanForDigestPropagatesOperationalFetchFailures(t *testing.T) {
	reg := newRegistry(t, nil)
	writer := newRegistryRepo(t, reg)
	published := publish(t, writer, publishSpec{content: "scan target"})
	byRecord := artifact.Ref{Name: appName, Digest: published.Pinned.RecordDigest}

	t.Run("a failing registry is not a missing digest", func(t *testing.T) {
		reader := newRegistryRepo(t, reg)
		reader.be = &unavailableBackend{backend: reader.be}
		_, err := reader.Resolve(context.Background(), byRecord)
		require.ErrorIs(t, err, errUtils.ErrArtifactDownloadFailed)
		require.NotErrorIs(t, err, errUtils.ErrArtifactNotFound)
		assert.ErrorContains(t, err, errServiceUnavailable.Error())
	})

	t.Run("a healthy registry still finds the Record", func(t *testing.T) {
		reader := newRegistryRepo(t, reg)
		pinned, err := reader.Resolve(context.Background(), byRecord)
		require.NoError(t, err)
		assert.Equal(t, published.Pinned.ResolvedDigest, pinned.ResolvedDigest)
	})

	t.Run("an unrelated tag that fails verification does not hide the match", func(t *testing.T) {
		other := publish(t, writer, publishSpec{version: "0.9.0", content: "different content"})
		reader := newRegistryRepo(t, reg)
		reader.be = &tamperedBackend{backend: reader.be, target: other.Pinned.ResolvedDigest}
		pinned, err := reader.Resolve(context.Background(), byRecord)
		require.NoError(t, err, "a tag whose manifest fails verification is a nonmatch, not a failure of the lookup")
		assert.Equal(t, published.Pinned.ResolvedDigest, pinned.ResolvedDigest)
	})

	t.Run("an absent digest is still not found", func(t *testing.T) {
		reader := newRegistryRepo(t, reg)
		_, err := reader.Resolve(context.Background(), artifact.Ref{Name: appName, Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"})
		require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	})
}
