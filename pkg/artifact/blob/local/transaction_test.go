package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

func TestManagedReadWaitsForBothCommitRenames(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	_, err := s.Put(ctx, "key", bytes.NewBufferString("old"), blob.PutOptions{Size: 3, Metadata: map[string]string{"version": "old"}})
	require.NoError(t, err)
	root, err := s.openRoot(false)
	require.NoError(t, err)
	pending, err := s.stage(ctx, root, "key", bytes.NewBufferString("new"), blob.PutOptions{Size: 3, Metadata: map[string]string{"version": "new"}})
	require.NoError(t, err)
	// commitJoined reports whether the commit goroutine ended. The root and staging area are torn
	// down only after it did, so a commit that is still running never sees a closed root.
	commitJoined := true
	t.Cleanup(func() {
		if !commitJoined {
			return
		}
		pending.discard(root)
		_ = root.Close()
	})
	release := holdGuard(t, s, "key", false)
	between, proceed := make(chan struct{}), make(chan struct{})
	// The commit stays parked on proceed until the test releases it. Release it on every exit so an
	// assertion that aborts the test never strands the commit goroutine holding the root lock.
	var proceedOnce sync.Once
	letProceed := func() { proceedOnce.Do(func() { close(proceed) }) }
	commitDone, commitFinished := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(commitFinished)
		defer release()
		commitDone <- s.commitManaged(root, "key", pending, false, func(r *os.Root, from, to string) error {
			if err := rootRenameReplace(r, from, to); err != nil {
				return err
			}
			if to == attrsPath("key") {
				close(between)
				<-proceed
			}
			return nil
		})
	}()
	// Registered after the goroutine starts, so it runs before the root teardown above: open the
	// gate, then wait for the commit to finish within a bound.
	commitJoined = false
	t.Cleanup(func() {
		letProceed()
		select {
		case <-commitFinished:
			commitJoined = true
		case <-time.After(waitTimeout):
			t.Errorf("timed out waiting for the commit goroutine to finish; leaving its root open")
		}
	})
	select {
	case <-between:
	case err := <-commitDone:
		t.Fatalf("the commit ended before its attribute rename: %v", err)
	case <-time.After(waitTimeout):
		t.Fatal("timed out waiting for the commit to reach its attribute rename")
	}
	type result struct {
		body string
		info *blob.ObjectInfo
		err  error
	}
	getDone, statDone := make(chan result, 1), make(chan result, 1)
	go func() {
		obj, err := s.Get(ctx, "key", blob.GetOptions{})
		if err != nil {
			getDone <- result{err: err}
			return
		}
		body, err := io.ReadAll(obj.Body)
		_ = obj.Body.Close()
		getDone <- result{body: string(body), info: &obj.ObjectInfo, err: err}
	}()
	go func() {
		info, err := s.Stat(ctx, "key", blob.StatOptions{})
		statDone <- result{info: info, err: err}
	}()
	early := false
	select {
	case got := <-getDone:
		t.Errorf("Get returned during an incomplete commit: %+v", got)
		early = true
	case got := <-statDone:
		t.Errorf("Stat returned during an incomplete commit: %+v", got)
		early = true
	case <-time.After(100 * time.Millisecond):
	}
	letProceed()
	require.NoError(t, receive(t, commitDone, "the commit to finish"))
	require.False(t, early)
	got, stat := receive(t, getDone, "Get to finish"), receive(t, statDone, "Stat to finish")
	require.NoError(t, got.err)
	require.NoError(t, stat.err)
	assert.Equal(t, "new", got.body)
	assert.Equal(t, "new", got.info.Metadata["version"])
	assert.Equal(t, "new", stat.info.Metadata["version"])
}

func TestManagedCommitFailureRestoresOldAttributes(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	_, err := s.Put(ctx, "key", bytes.NewBufferString("old"), blob.PutOptions{Size: 3, Metadata: map[string]string{"version": "old"}})
	require.NoError(t, err)
	root, err := s.openRoot(false)
	require.NoError(t, err)
	defer root.Close()
	pending, err := s.stage(ctx, root, "key", bytes.NewBufferString("new"), blob.PutOptions{Size: 3, Metadata: map[string]string{"version": "new"}})
	require.NoError(t, err)
	defer pending.discard(root)
	release := holdGuard(t, s, "key", false)
	failure := errors.New("injected data rename failure")
	err = s.commitManaged(root, "key", pending, false, func(r *os.Root, from, to string) error {
		if to == "key" {
			return failure
		}
		return rootRenameReplace(r, from, to)
	})
	release()
	require.ErrorIs(t, err, failure)
	obj, err := s.Get(ctx, "key", blob.GetOptions{})
	require.NoError(t, err)
	defer obj.Body.Close()
	body, err := io.ReadAll(obj.Body)
	require.NoError(t, err)
	assert.Equal(t, "old", string(body))
	assert.Equal(t, "old", obj.Metadata["version"])
	assert.NoFileExists(t, filepath.Join(s.root, transactionName))
}

func TestManagedRecoveryAfterInterruptedCommit(t *testing.T) {
	for _, dataRenamed := range []bool{false, true} {
		t.Run(map[bool]string{false: "before data rename", true: "after data rename"}[dataRenamed], func(t *testing.T) {
			ctx := context.Background()
			s := newStore(t)
			_, err := s.Put(ctx, "key", bytes.NewBufferString("old"), blob.PutOptions{Size: 3, Metadata: map[string]string{"version": "old"}})
			require.NoError(t, err)
			root, err := s.openRoot(false)
			require.NoError(t, err)
			closeOnCleanup(t, root)
			pending, err := s.stage(ctx, root, "key", bytes.NewBufferString("new"), blob.PutOptions{Size: 3, Metadata: map[string]string{"version": "new"}})
			require.NoError(t, err)
			oldAttrs, err := rootReadFile(root, attrsPath("key"))
			require.NoError(t, err)
			marker, err := json.Marshal(transaction{Target: "key", DataTemp: pending.tempPath, AttrsTemp: pending.attrsTemp, HadAttrs: true, OldAttrs: oldAttrs})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(s.root, transactionName), marker, 0o644))
			require.NoError(t, rootRenameReplace(root, pending.attrsTemp, attrsPath("key")))
			if dataRenamed {
				require.NoError(t, rootRenameReplace(root, pending.tempPath, "key"))
			}
			require.NoError(t, root.Close())
			restarted, err := New(s.root)
			require.NoError(t, err)
			obj, err := restarted.Get(ctx, "key", blob.GetOptions{})
			require.NoError(t, err)
			defer obj.Body.Close()
			body, err := io.ReadAll(obj.Body)
			require.NoError(t, err)
			want := "old"
			if dataRenamed {
				want = "new"
			}
			assert.Equal(t, want, string(body))
			assert.Equal(t, want, obj.Metadata["version"])
			assert.NoFileExists(t, filepath.Join(s.root, transactionName))
		})
	}
}

func TestManagedRecoveryRemovesNewAttributesWhenOldObjectHadNone(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	_, err := s.Put(ctx, "key", bytes.NewBufferString("old"), blob.PutOptions{Size: 3})
	require.NoError(t, err)
	root, err := s.openRoot(false)
	require.NoError(t, err)
	closeOnCleanup(t, root)
	pending, err := s.stage(ctx, root, "key", bytes.NewBufferString("new"), blob.PutOptions{Size: 3, Metadata: map[string]string{"version": "new"}})
	require.NoError(t, err)
	marker, err := json.Marshal(transaction{Target: "key", DataTemp: pending.tempPath, AttrsTemp: pending.attrsTemp})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(s.root, transactionName), marker, 0o644))
	require.NoError(t, rootRenameReplace(root, pending.attrsTemp, attrsPath("key")))
	require.NoError(t, root.Close())
	info, err := s.Stat(ctx, "key", blob.StatOptions{})
	require.NoError(t, err)
	assert.Empty(t, info.Metadata)
	assert.NoFileExists(t, filepath.Join(s.root, attrsPath("key")))
	assert.NoFileExists(t, filepath.Join(s.root, transactionName))
}
