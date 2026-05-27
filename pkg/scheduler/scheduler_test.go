package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunExecutesLinearChainInDependencyOrder(t *testing.T) {
	graph := testGraph(t, map[string][]string{
		"a": nil,
		"b": {"a"},
		"c": {"b"},
	})

	var mu sync.Mutex
	started := make([]string, 0, 3)
	dispatcher := DispatcherFunc(func(_ context.Context, node *dependency.Node) (Result, error) {
		mu.Lock()
		started = append(started, node.ID)
		mu.Unlock()
		return Result{}, nil
	})

	result := runWithTimeout(t, New(graph, dispatcher, WithMaxConcurrency(4)))

	require.NoError(t, result.Err)
	assert.True(t, result.Success())
	assert.Equal(t, []string{"a", "b", "c"}, started)
	assertResultOrder(t, result, "a", "b", "c")
	assertStatuses(t, result, map[string]Status{
		"a": StatusSucceeded,
		"b": StatusSucceeded,
		"c": StatusSucceeded,
	})
}

func TestRunExecutesSingleNode(t *testing.T) {
	graph := testGraph(t, map[string][]string{
		"a": nil,
	})

	result := runWithTimeout(t, New(graph, DispatcherFunc(func(_ context.Context, node *dependency.Node) (Result, error) {
		return Result{Value: node.ID}, nil
	})))

	require.NoError(t, result.Err)
	require.Len(t, result.Results, 1)
	assert.True(t, result.Success())
	assert.Equal(t, "a", result.Results[0].NodeID)
	assert.Equal(t, StatusSucceeded, result.Results[0].Status)
	assert.Equal(t, "a", result.Results[0].Value)
}

func TestRunExecutesDiamondDAGInStableOrder(t *testing.T) {
	graph := testGraph(t, map[string][]string{
		"a": nil,
		"b": {"a"},
		"c": {"a"},
		"d": {"b", "c"},
	})

	result := runWithTimeout(t, New(graph, successfulDispatcher(), WithMaxConcurrency(2)))

	require.NoError(t, result.Err)
	assertResultOrder(t, result, "a", "b", "c", "d")
	assertStatuses(t, result, map[string]Status{
		"a": StatusSucceeded,
		"b": StatusSucceeded,
		"c": StatusSucceeded,
		"d": StatusSucceeded,
	})
}

func TestRunExecutesFanOutDAGInStableOrder(t *testing.T) {
	graph := testGraph(t, map[string][]string{
		"a": nil,
		"b": {"a"},
		"c": {"a"},
		"d": {"a"},
	})

	result := runWithTimeout(t, New(graph, successfulDispatcher(), WithMaxConcurrency(3)))

	require.NoError(t, result.Err)
	assertResultOrder(t, result, "a", "b", "c", "d")
	assertStatuses(t, result, map[string]Status{
		"a": StatusSucceeded,
		"b": StatusSucceeded,
		"c": StatusSucceeded,
		"d": StatusSucceeded,
	})
}

func TestRunExecutesFanInDAGOnlyAfterAllDependenciesComplete(t *testing.T) {
	graph := testGraph(t, map[string][]string{
		"a": nil,
		"b": nil,
		"c": nil,
		"d": {"a", "b", "c"},
	})

	releaseA := make(chan struct{})
	releaseB := make(chan struct{})
	started := make(chan string, 4)
	dispatcher := DispatcherFunc(func(ctx context.Context, node *dependency.Node) (Result, error) {
		started <- node.ID
		switch node.ID {
		case "a":
			select {
			case <-releaseA:
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		case "b":
			select {
			case <-releaseB:
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		}
		return Result{}, nil
	})

	done := make(chan *AggregateResult, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		done <- New(graph, dispatcher, WithMaxConcurrency(4)).Run(ctx)
	}()

	seen := waitForStartedSet(t, started, nil, "a", "b", "c")
	assertNoStart(t, started, 50*time.Millisecond)
	close(releaseA)
	assertNoStart(t, started, 50*time.Millisecond)
	close(releaseB)
	waitForStartedSet(t, started, seen, "d")

	result := <-done
	require.NoError(t, result.Err)
	assertResultOrder(t, result, "a", "b", "c", "d")
}

func TestRunUsesReadyQueueWithoutWaitingForUnrelatedRoot(t *testing.T) {
	graph := testGraph(t, map[string][]string{
		"a": nil,
		"b": nil,
		"c": {"b"},
	})

	releaseA := make(chan struct{})
	started := make(chan string, 3)
	dispatcher := DispatcherFunc(func(ctx context.Context, node *dependency.Node) (Result, error) {
		started <- node.ID
		if node.ID == "a" {
			select {
			case <-releaseA:
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		}
		return Result{}, nil
	})

	done := make(chan *AggregateResult, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		done <- New(graph, dispatcher, WithMaxConcurrency(2)).Run(ctx)
	}()

	seen := waitForStartedSet(t, started, nil, "a", "b")
	waitForStartedSet(t, started, seen, "c")
	close(releaseA)

	result := <-done
	require.NoError(t, result.Err)
	assertResultOrder(t, result, "a", "b", "c")
}

func TestRunBoundsConcurrentWorkers(t *testing.T) {
	graph := testGraph(t, map[string][]string{
		"a": nil,
		"b": nil,
		"c": nil,
		"d": nil,
	})

	release := make(chan struct{})
	started := make(chan string, 4)
	var mu sync.Mutex
	active := 0
	maxActive := 0
	dispatcher := DispatcherFunc(func(ctx context.Context, node *dependency.Node) (Result, error) {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()

		started <- node.ID
		select {
		case <-release:
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}

		mu.Lock()
		active--
		mu.Unlock()
		return Result{}, nil
	})

	done := make(chan *AggregateResult, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		done <- New(graph, dispatcher, WithMaxConcurrency(2)).Run(ctx)
	}()

	waitForAnyStart(t, started)
	waitForAnyStart(t, started)
	assertNoStart(t, started, 50*time.Millisecond)

	close(release)
	result := <-done
	require.NoError(t, result.Err)
	assert.Equal(t, 2, maxActive)
}

func TestRunReturnsDeterministicAggregateResults(t *testing.T) {
	graph := testGraph(t, map[string][]string{
		"a": nil,
		"b": nil,
		"c": nil,
	})

	dispatcher := DispatcherFunc(func(_ context.Context, node *dependency.Node) (Result, error) {
		switch node.ID {
		case "a":
			time.Sleep(30 * time.Millisecond)
		case "b":
			time.Sleep(20 * time.Millisecond)
		case "c":
			time.Sleep(10 * time.Millisecond)
		}
		return Result{Value: node.ID}, nil
	})

	result := runWithTimeout(t, New(graph, dispatcher, WithMaxConcurrency(3)))

	require.NoError(t, result.Err)
	assertResultOrder(t, result, "a", "b", "c")
	for _, nodeResult := range result.Results {
		assert.Equal(t, nodeResult.NodeID, nodeResult.Value)
	}
}

func TestRunKeepGoingSkipsBlockedDependentsAndRunsIndependentNodes(t *testing.T) {
	graph := testGraph(t, map[string][]string{
		"a": nil,
		"b": {"a"},
		"c": {"b"},
		"z": nil,
	})

	boom := errors.New("boom")
	started := map[string]bool{}
	var mu sync.Mutex
	dispatcher := DispatcherFunc(func(_ context.Context, node *dependency.Node) (Result, error) {
		mu.Lock()
		started[node.ID] = true
		mu.Unlock()
		if node.ID == "a" {
			return Result{}, boom
		}
		return Result{}, nil
	})

	result := runWithTimeout(t, New(graph, dispatcher, WithMaxConcurrency(2)))

	require.ErrorIs(t, result.Err, boom)
	assertStatuses(t, result, map[string]Status{
		"a": StatusFailed,
		"b": StatusSkipped,
		"c": StatusSkipped,
		"z": StatusSucceeded,
	})
	assert.True(t, started["a"])
	assert.True(t, started["z"])
	assert.False(t, started["b"])
	assert.False(t, started["c"])
}

func TestRunFailFastSkipsPendingWorkAfterFirstError(t *testing.T) {
	graph := testGraph(t, map[string][]string{
		"a": nil,
		"b": nil,
		"c": nil,
	})

	boom := errors.New("boom")
	started := map[string]bool{}
	dispatcher := DispatcherFunc(func(_ context.Context, node *dependency.Node) (Result, error) {
		started[node.ID] = true
		if node.ID == "a" {
			return Result{}, boom
		}
		return Result{}, nil
	})

	result := runWithTimeout(t, New(graph, dispatcher, WithMaxConcurrency(1), WithFailFast(true)))

	require.ErrorIs(t, result.Err, boom)
	assertStatuses(t, result, map[string]Status{
		"a": StatusFailed,
		"b": StatusSkipped,
		"c": StatusSkipped,
	})
	assert.True(t, started["a"])
	assert.False(t, started["b"])
	assert.False(t, started["c"])
}

func TestRunFailFastLetsAlreadyRunningNodesDrain(t *testing.T) {
	graph := testGraph(t, map[string][]string{
		"a": nil,
		"b": nil,
		"c": {"b"},
	})

	boom := errors.New("boom")
	releaseA := make(chan struct{})
	releaseB := make(chan struct{})
	started := make(chan string, 2)
	dispatcher := DispatcherFunc(func(ctx context.Context, node *dependency.Node) (Result, error) {
		started <- node.ID
		switch node.ID {
		case "a":
			select {
			case <-releaseA:
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
			return Result{}, boom
		case "b":
			select {
			case <-releaseB:
				return Result{}, nil
			case <-ctx.Done():
				return Result{}, ctx.Err()
			}
		default:
			return Result{}, nil
		}
	})

	done := make(chan *AggregateResult, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		done <- New(graph, dispatcher, WithMaxConcurrency(2), WithFailFast(true)).Run(ctx)
	}()

	waitForStartedSet(t, started, nil, "a", "b")
	close(releaseA)
	assertNoResult(t, done, 50*time.Millisecond)
	close(releaseB)

	result := <-done
	require.ErrorIs(t, result.Err, boom)
	require.NotErrorIs(t, result.Err, context.Canceled)
	assertStatuses(t, result, map[string]Status{
		"a": StatusFailed,
		"b": StatusSucceeded,
		"c": StatusSkipped,
	})
}

func TestRunPropagatesSkippedStatusToDependents(t *testing.T) {
	graph := testGraph(t, map[string][]string{
		"a": nil,
		"b": {"a"},
		"c": {"b"},
		"z": nil,
	})

	skipErr := errors.New("skip a")
	dispatcher := DispatcherFunc(func(_ context.Context, node *dependency.Node) (Result, error) {
		if node.ID == "a" {
			return Result{Status: StatusSkipped, Err: skipErr}, nil
		}
		return Result{}, nil
	})

	result := runWithTimeout(t, New(graph, dispatcher, WithMaxConcurrency(2)))

	require.ErrorIs(t, result.Err, skipErr)
	require.ErrorIs(t, result.Err, errUtils.ErrNodeSkipped)
	assertStatuses(t, result, map[string]Status{
		"a": StatusSkipped,
		"b": StatusSkipped,
		"c": StatusSkipped,
		"z": StatusSucceeded,
	})
	for _, id := range []string{"b", "c"} {
		nodeResult, ok := result.ResultFor(id)
		require.True(t, ok)
		require.ErrorIs(t, nodeResult.Err, skipErr)
		require.ErrorIs(t, nodeResult.Err, errUtils.ErrNodeSkipped)
	}
}

func TestRunValidatesInputs(t *testing.T) {
	result := New(nil, DispatcherFunc(func(context.Context, *dependency.Node) (Result, error) {
		return Result{}, nil
	})).Run(context.Background())
	require.ErrorIs(t, result.Err, errUtils.ErrNilGraph)

	graph := testGraph(t, map[string][]string{"a": nil})
	result = New(graph, nil).Run(context.Background())
	require.ErrorIs(t, result.Err, errUtils.ErrNilDispatcher)

	result = New(graph, DispatcherFunc(func(context.Context, *dependency.Node) (Result, error) {
		return Result{}, nil
	}), WithMaxConcurrency(0)).Run(context.Background())
	require.ErrorIs(t, result.Err, errUtils.ErrInvalidWorker)
}

func TestRunRejectsCyclicGraph(t *testing.T) {
	graph := dependency.NewGraph()
	require.NoError(t, graph.AddNode(&dependency.Node{ID: "a"}))
	require.NoError(t, graph.AddNode(&dependency.Node{ID: "b"}))
	require.NoError(t, graph.AddDependency("a", "b"))
	require.NoError(t, graph.AddDependency("b", "a"))

	result := New(graph, successfulDispatcher()).Run(context.Background())

	require.ErrorIs(t, result.Err, errUtils.ErrInvalidGraph)
	require.ErrorIs(t, result.Err, dependency.ErrCircularDependency)
}

func TestRunInvokesNodeHooks(t *testing.T) {
	graph := testGraph(t, map[string][]string{
		"a": nil,
		"b": {"a"},
	})

	var mu sync.Mutex
	started := make([]string, 0, 2)
	completed := make([]string, 0, 2)
	result := runWithTimeout(t, New(
		graph,
		successfulDispatcher(),
		WithNodeStartHook(func(node *dependency.Node) {
			mu.Lock()
			defer mu.Unlock()
			started = append(started, node.ID)
		}),
		WithNodeCompleteHook(func(node *dependency.Node, result Result) {
			mu.Lock()
			defer mu.Unlock()
			completed = append(completed, node.ID+":"+string(result.Status))
		}),
	))

	require.NoError(t, result.Err)
	assert.Equal(t, []string{"a", "b"}, started)
	assert.Equal(t, []string{"a:succeeded", "b:succeeded"}, completed)
}

func successfulDispatcher() Dispatcher {
	return DispatcherFunc(func(_ context.Context, _ *dependency.Node) (Result, error) {
		return Result{}, nil
	})
}

func testGraph(t *testing.T, deps map[string][]string) *dependency.Graph {
	t.Helper()

	builder := dependency.NewBuilder()
	for id := range deps {
		require.NoError(t, builder.AddNode(&dependency.Node{ID: id, Component: id, Stack: "test", Type: "test"}))
	}
	for id, dependencies := range deps {
		for _, dep := range dependencies {
			require.NoError(t, builder.AddDependency(id, dep))
		}
	}
	graph, err := builder.Build()
	require.NoError(t, err)
	return graph
}

func runWithTimeout(t *testing.T, scheduler *Scheduler) *AggregateResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return scheduler.Run(ctx)
}

func assertResultOrder(t *testing.T, result *AggregateResult, ids ...string) {
	t.Helper()

	got := make([]string, 0, len(result.Results))
	for _, nodeResult := range result.Results {
		got = append(got, nodeResult.NodeID)
	}
	assert.Equal(t, ids, got)
}

func assertStatuses(t *testing.T, result *AggregateResult, expected map[string]Status) {
	t.Helper()

	for id, status := range expected {
		nodeResult, ok := result.ResultFor(id)
		require.True(t, ok, "missing result for %s", id)
		assert.Equal(t, status, nodeResult.Status, "status for %s", id)
	}
}

func waitForStartedSet(t *testing.T, started <-chan string, seen map[string]bool, wants ...string) map[string]bool {
	t.Helper()

	if seen == nil {
		seen = map[string]bool{}
	}
	deadline := time.After(time.Second)
	for {
		allSeen := true
		for _, want := range wants {
			if !seen[want] {
				allSeen = false
				break
			}
		}
		if allSeen {
			return seen
		}

		select {
		case got := <-started:
			seen[got] = true
		case <-deadline:
			t.Fatalf("timed out waiting for nodes to start: %v", wants)
		}
	}
}

func waitForAnyStart(t *testing.T, started <-chan string) {
	t.Helper()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for node to start")
	}
}

func assertNoStart(t *testing.T, started <-chan string, duration time.Duration) {
	t.Helper()

	select {
	case id := <-started:
		t.Fatalf("unexpected node start while workers were saturated: %s", id)
	case <-time.After(duration):
	}
}

func assertNoResult(t *testing.T, done <-chan *AggregateResult, duration time.Duration) {
	t.Helper()

	select {
	case result := <-done:
		t.Fatalf("scheduler finished before running workers drained: %#v", result)
	case <-time.After(duration):
	}
}
