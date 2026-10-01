package ordered

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for batch progress")
		var zero T
		return zero
	}
}

func TestRunBoundedPreparationAndOrderedCommit(t *testing.T) {
	const count, concurrency = 10, 2
	started := make(chan int, count)
	releases := make([]chan struct{}, count)
	for i := range releases {
		releases[i] = make(chan struct{})
	}
	var active, maximum atomic.Int32
	var committed, disposed []int
	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Options[int]{
			Count: count, Concurrency: concurrency,
			Prepare: func(_ context.Context, i int) (int, error) {
				current := active.Add(1)
				for old := maximum.Load(); current > old && !maximum.CompareAndSwap(old, current); old = maximum.Load() {
				}
				started <- i
				<-releases[i]
				active.Add(-1)
				return i, nil
			},
			Commit: func(i, value int, err error) error {
				assert.NoError(t, err)
				assert.Equal(t, i, value)
				committed = append(committed, i)
				return nil
			},
			Dispose: func(value int) { disposed = append(disposed, value) },
		})
	}()
	first := []int{receive(t, started), receive(t, started)}
	require.ElementsMatch(t, []int{0, 1}, first)
	// Hold the first package while later packages finish in reverse commit order.
	close(releases[1])
	require.Equal(t, 2, receive(t, started))
	close(releases[2])
	require.Equal(t, 3, receive(t, started))
	close(releases[3])
	select {
	case i := <-started:
		t.Fatalf("prepared %d beyond the twice-worker staging window", i)
	case <-time.After(30 * time.Millisecond):
	}
	close(releases[0])
	for range count - 4 {
		i := receive(t, started)
		close(releases[i])
	}
	require.NoError(t, receive(t, done))
	expected := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	assert.Equal(t, expected, committed)
	assert.Equal(t, expected, disposed)
	assert.Equal(t, int32(concurrency), maximum.Load())
}

func TestRunSerialReadsAndBarrierObserveEarlierCommits(t *testing.T) {
	for _, concurrency := range []int{1, 4} {
		t.Run(fmt.Sprint(concurrency), func(t *testing.T) {
			var latest atomic.Int32
			latest.Store(-1)
			var committed []int
			require.NoError(t, Run(context.Background(), Options[int]{
				Count: 4, Concurrency: concurrency,
				Barrier: func(i int) bool { return i == 2 },
				Prepare: func(_ context.Context, i int) (int, error) {
					if concurrency == 1 || i == 2 {
						assert.Equal(t, int32(i-1), latest.Load(), "source must see preceding writes")
					}
					return i, nil
				},
				Commit: func(i, value int, err error) error {
					committed = append(committed, value)
					latest.Store(int32(i))
					return nil
				},
			}))
			assert.Equal(t, []int{0, 1, 2, 3}, committed)
		})
	}
}

func TestRunPreparationFailureCanContinue(t *testing.T) {
	fetchErr := errors.New("fetch failed")
	var values []int
	require.NoError(t, Run(context.Background(), Options[int]{
		Count: 3, Concurrency: 2,
		Prepare: func(_ context.Context, i int) (int, error) {
			if i == 1 {
				return i, fetchErr
			}
			return i, nil
		},
		Commit: func(i, value int, err error) error {
			if i == 1 {
				assert.ErrorIs(t, err, fetchErr)
			} else {
				assert.NoError(t, err)
			}
			values = append(values, value)
			return nil
		},
	}))
	assert.Equal(t, []int{0, 1, 2}, values)
}

func TestRunCommitFailureDisposesPreparedResultsAndJoinsWorkers(t *testing.T) {
	commitErr := errors.New("receipt failed")
	ready := make(chan struct{})
	var prepared atomic.Int32
	var disposed []int
	var commits []int
	require.ErrorIs(t, Run(context.Background(), Options[int]{
		Count: 3, Concurrency: 3,
		Prepare: func(_ context.Context, i int) (int, error) {
			if prepared.Add(1) == 3 {
				close(ready)
			}
			<-ready
			return i, nil
		},
		Commit:  func(i, value int, err error) error { commits = append(commits, i); return commitErr },
		Dispose: func(value int) { disposed = append(disposed, value) },
	}), commitErr)
	assert.Equal(t, []int{0}, commits)
	assert.ElementsMatch(t, []int{0, 1, 2}, disposed)
}

func TestRunCancellationDisposesWorkersResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan int, 2)
	done := make(chan error, 1)
	var prepared, disposed []int
	var mu sync.Mutex
	go func() {
		done <- Run(ctx, Options[int]{
			Count: 20, Concurrency: 2,
			Prepare: func(ctx context.Context, i int) (int, error) {
				mu.Lock()
				prepared = append(prepared, i)
				mu.Unlock()
				started <- i
				<-ctx.Done()
				return i, ctx.Err()
			},
			Commit:  func(i, value int, err error) error { t.Errorf("committed canceled package %d", i); return nil },
			Dispose: func(value int) { disposed = append(disposed, value) },
		})
	}()
	receive(t, started)
	receive(t, started)
	cancel()
	require.ErrorIs(t, receive(t, done), context.Canceled)
	assert.ElementsMatch(t, prepared, disposed)
	assert.Len(t, disposed, 2)
}

func TestRunValidationAndEmptyBatch(t *testing.T) {
	for _, concurrency := range []int{-1, 0} {
		require.ErrorIs(t, Run(context.Background(), Options[int]{Concurrency: concurrency}), errUtils.ErrInvalidFlagValue)
	}
	require.NoError(t, Run(context.Background(), Options[int]{Concurrency: 1}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, Run(ctx, Options[int]{Concurrency: 1}), context.Canceled)
	require.ErrorIs(t, Run(ctx, Options[int]{Count: 4, Concurrency: 2, Prepare: func(context.Context, int) (int, error) { t.Error("prepared after cancellation"); return 0, nil }}), context.Canceled)
}

func TestRunCancellationDuringFinalCommitFinishesTransaction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var transactionFinished, disposed bool
	err := Run(ctx, Options[int]{
		Count: 1, Concurrency: 1,
		Prepare: func(context.Context, int) (int, error) { return 42, nil },
		Commit:  func(i, value int, err error) error { cancel(); transactionFinished = true; return nil },
		Dispose: func(value int) { assert.True(t, transactionFinished); assert.Equal(t, 42, value); disposed = true },
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.True(t, transactionFinished)
	assert.True(t, disposed)
}

// BenchmarkRunPreparation measures an eight-source fetch-bound workload without
// imposing a noisy wall-clock performance threshold on unit tests.
func BenchmarkRunPreparation(b *testing.B) {
	for _, concurrency := range []int{1, 4} {
		b.Run(fmt.Sprintf("workers-%d", concurrency), func(b *testing.B) {
			for b.Loop() {
				err := Run(context.Background(), Options[int]{
					Count: 8, Concurrency: concurrency,
					Prepare: func(context.Context, int) (int, error) { time.Sleep(2 * time.Millisecond); return 1, nil },
					Commit:  func(int, int, error) error { return nil },
				})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
