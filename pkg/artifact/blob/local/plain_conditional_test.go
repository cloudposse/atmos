package local

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

func putBody(t *testing.T, s *Store, key, body string, cond blob.Condition) error {
	t.Helper()
	_, err := s.Put(context.Background(), key, bytes.NewReader([]byte(body)), blob.PutOptions{Size: int64(len(body)), Condition: cond})
	return err
}

func readBody(t *testing.T, s *Store, key string) string {
	t.Helper()
	obj, err := s.Get(context.Background(), key, blob.GetOptions{})
	require.NoError(t, err)
	defer obj.Body.Close()
	data, err := io.ReadAll(obj.Body)
	require.NoError(t, err)
	return string(data)
}

// A plain-file store leaves unconditional writes unlocked for legacy writers, so it cannot make a
// conditional write atomic against them. It must say so instead of advertising and accepting
// conditions it cannot enforce.
func TestPlainFileStoreWithoutLockAllRefusesConditionalOperations(t *testing.T) {
	s := newStore(t, WithPlainFileCompatibility())
	require.NoError(t, putBody(t, s, "k", "A", blob.Condition{}))
	info, err := s.Stat(context.Background(), "k", blob.StatOptions{})
	require.NoError(t, err)

	caps := s.Capabilities()
	assert.False(t, caps.ConditionalPut, "conditional writes are not advertised")
	assert.False(t, caps.ConditionalDelete, "conditional deletes are not advertised")

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "if-absent put", run: func() error { return putBody(t, s, "new", "B", blob.IfAbsent()) }},
		{name: "if-match put", run: func() error { return putBody(t, s, "k", "B", blob.IfMatch(info.ETag)) }},
		{name: "if-match delete", run: func() error {
			return s.Delete(context.Background(), "k", blob.DeleteOptions{IfMatch: info.ETag})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			require.ErrorIs(t, err, errUtils.ErrBlobConditionalUnsupported)
			assert.True(t, blob.IsConditionalUnsupported(err))
		})
	}
	assert.Equal(t, "A", readBody(t, s, "k"), "a refused conditional operation changes nothing")
	_, err = s.Stat(context.Background(), "new", blob.StatOptions{})
	require.ErrorIs(t, err, errUtils.ErrBlobNotFound)
	assert.NoFileExists(t, s.Root()+"/"+lockName, "refusing takes no lock")

	require.NoError(t, putBody(t, s, "k", "C", blob.Condition{}), "unconditional writes keep working")
	require.NoError(t, s.Delete(context.Background(), "k", blob.DeleteOptions{}), "unconditional deletes keep working")
}

// With WithLockAllWrites a plain-file store serializes every mutation, so a conditional operation
// is atomic against unconditional ones and the capabilities are honest.
func TestPlainFileStoreWithLockAllEnforcesConditionsAgainstUnconditionalWrites(t *testing.T) {
	s := newStore(t, WithPlainFileCompatibility(), WithLockAllWrites())
	assert.True(t, s.Capabilities().ConditionalPut)
	assert.True(t, s.Capabilities().ConditionalDelete)

	t.Run("unconditional mutations wait for a held root lock", func(t *testing.T) {
		require.NoError(t, putBody(t, s, "held", "A", blob.Condition{}))
		release := holdGuard(t, s, "held", true)

		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		_, putErr := s.Put(ctx, "held", bytes.NewReader([]byte("C")), blob.PutOptions{Size: 1})
		require.ErrorIs(t, putErr, context.DeadlineExceeded, "an unconditional put cannot slip between a check and a commit")
		ctx2, cancel2 := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel2()
		deleteErr := s.Delete(ctx2, "held", blob.DeleteOptions{})
		require.ErrorIs(t, deleteErr, context.DeadlineExceeded, "an unconditional delete cannot slip in either")

		release()
		assert.Equal(t, "A", readBody(t, s, "held"), "nothing was written while the lock was held")
		require.NoError(t, putBody(t, s, "held", "C", blob.Condition{}))
	})

	t.Run("an if-match put never overwrites a competing unconditional put", func(t *testing.T) {
		for round := 0; round < 50; round++ {
			key := "race"
			require.NoError(t, putBody(t, s, key, "A", blob.Condition{}))
			info, err := s.Stat(context.Background(), key, blob.StatOptions{})
			require.NoError(t, err)

			var wg sync.WaitGroup
			start := make(chan struct{})
			var conditionalErr error
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				conditionalErr = putBody(t, s, key, "B", blob.IfMatch(info.ETag))
			}()
			go func() {
				defer wg.Done()
				<-start
				assert.NoError(t, putBody(t, s, key, "C", blob.Condition{}))
			}()
			close(start)
			finished := make(chan struct{})
			go func() {
				wg.Wait()
				close(finished)
			}()
			receive(t, finished, "both writers to finish")

			if conditionalErr != nil {
				require.ErrorIs(t, conditionalErr, errUtils.ErrBlobPreconditionFailed)
			}
			assert.Equal(t, "C", readBody(t, s, key), "round %d: the conditional write either ran before C or failed", round)
		}
	})
}
