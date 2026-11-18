package oci

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/filelock"
)

// layoutRepoWithGuardDir opens a layout repository whose write guards live in lockDir. Repositories
// that share a layout path and a lock directory behave like separate Atmos processes on one host.
func layoutRepoWithGuardDir(t *testing.T, path, lockDir string) *Repository {
	t.Helper()
	repo, err := NewRepository(context.Background(), "layout", layoutConfig(path), WithOwner(testOwner), WithLockDir(lockDir))
	require.NoError(t, err)
	return repo
}

// The layout write guard is scoped to the layout root: a writer of a/app and a writer of b/app share
// one index.json, so they must exclude each other even though their reference locks differ.
func TestLayoutGuardExcludesWritersOfDifferentReferences(t *testing.T) {
	path, lockDir := filepath.Join(t.TempDir(), "layout"), t.TempDir()
	holder := layoutRepoWithGuardDir(t, path, lockDir)
	writer := layoutRepoWithGuardDir(t, path, lockDir)
	existing := publish(t, holder, publishSpec{name: "a/app", content: "first"})

	release, err := holder.guardLayout(context.Background())
	require.NoError(t, err)

	// Every layout write path waits for the guard: publish, copy, delete, and attachments.
	timeout := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 100*time.Millisecond)
	}
	t.Run("publish", func(t *testing.T) {
		ctx, cancel := timeout()
		defer cancel()
		_, err := writer.Publish(ctx, prepare(t, publishSpec{name: "b/app", content: "second"}), artifact.PublishOptions{})
		require.ErrorIs(t, err, filelock.ErrAcquire)
		assert.False(t, resolves(writer, "b/app", appVersion), "nothing was written while the guard was held elsewhere")
	})
	t.Run("copy", func(t *testing.T) {
		source := newLayoutRepo(t)
		copied := publish(t, source, publishSpec{name: "c/app", content: "copied"})
		ctx, cancel := timeout()
		defer cancel()
		_, err := writer.CopyFrom(ctx, source, copied.Pinned, artifact.CopyFromOptions{})
		require.ErrorIs(t, err, filelock.ErrAcquire)
		assert.False(t, resolves(writer, "c/app", appVersion))
	})
	t.Run("delete", func(t *testing.T) {
		ctx, cancel := timeout()
		defer cancel()
		_, err := writer.Delete(ctx, existing.Pinned, artifact.DeleteOptions{Scope: artifact.DeleteReference})
		require.ErrorIs(t, err, filelock.ErrAcquire)
		assert.True(t, resolves(writer, "a/app", appVersion))
	})
	t.Run("attachment", func(t *testing.T) {
		st, loaded, err := writer.rootOf(context.Background(), &existing.Pinned)
		require.NoError(t, err)
		subject := artifact.SignatureSubject{Kind: artifact.SubjectOCIRoot, Digest: loaded.desc.Digest, MediaType: loaded.desc.MediaType, Size: loaded.desc.Size}
		payload := &artifact.EvidencePayload{Format: artifact.EvidenceSigstoreBundle, MediaType: bundleMediaTypePrefix + "v0.3+json", Data: []byte("bundle")}
		ctx, cancel := timeout()
		defer cancel()
		_, _, err = writer.storeAttachment(ctx, st, &attachRequest{root: loaded.desc, subject: subject, payload: payload})
		require.ErrorIs(t, err, filelock.ErrAcquire)
	})

	// A waiting writer proceeds as soon as the guard is released.
	done := make(chan error, 1)
	go func() {
		_, err := writer.Publish(context.Background(), prepare(t, publishSpec{name: "b/app", content: "second"}), artifact.PublishOptions{})
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("publish finished while the guard was held: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	release()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("publish did not proceed after the guard was released")
	}
	assert.True(t, resolves(writer, "a/app", appVersion))
	assert.True(t, resolves(writer, "b/app", appVersion))
}

// Concurrent writers of different references through separate Repository values never lose each
// other's index update: every successful publication is listed afterwards.
func TestLayoutConcurrentWritersKeepEveryReference(t *testing.T) {
	const writers = 8
	path, lockDir := filepath.Join(t.TempDir(), "layout"), t.TempDir()
	prepared := make([]*artifact.PreparedArtifact, writers)
	repos := make([]*Repository, writers)
	for index := range writers {
		repos[index] = layoutRepoWithGuardDir(t, path, lockDir)
		prepared[index] = prepare(t, publishSpec{name: fmt.Sprintf("team%d/app", index), content: fmt.Sprintf("content %d", index)})
	}

	var wait sync.WaitGroup
	errs := make([]error, writers)
	for index := range writers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, errs[index] = repos[index].Publish(context.Background(), prepared[index], artifact.PublishOptions{})
		}()
	}
	wait.Wait()
	for index, err := range errs {
		require.NoError(t, err, "writer %d", index)
	}

	reader := layoutRepoWithGuardDir(t, path, lockDir)
	for index := range writers {
		name := fmt.Sprintf("team%d/app", index)
		assert.True(t, resolves(reader, name, appVersion), "%s was lost by a concurrent writer", name)
	}
}

// A registry has no shared index file, so its guard never touches the lock directory.
func TestRegistryGuardIsANoOp(t *testing.T) {
	repo := newRegistryRepo(t, newRegistry(t, nil))
	release, err := repo.guardLayout(context.Background())
	require.NoError(t, err)
	release()
}
