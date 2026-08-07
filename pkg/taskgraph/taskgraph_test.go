package taskgraph

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/dependency"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// staticLookup returns a fixed dependency map: name -> its own direct Refs.
func staticLookup(deps map[string][]Ref) Lookup {
	return func(ref Ref) ([]Ref, bool, error) {
		children, ok := deps[ref.Name]
		if !ok {
			return nil, false, nil
		}
		return children, true, nil
	}
}

// countingRunner records every executed Ref (thread-safe) and returns nil.
func countingRunner(t *testing.T) (Runner, func() []Ref) {
	t.Helper()
	var mu sync.Mutex
	var ran []Ref
	runner := func(_ context.Context, ref Ref) error {
		mu.Lock()
		defer mu.Unlock()
		ran = append(ran, ref)
		return nil
	}
	return runner, func() []Ref {
		mu.Lock()
		defer mu.Unlock()
		out := make([]Ref, len(ran))
		copy(out, ran)
		return out
	}
}

func TestRun_DiamondDependencyExecutesSharedDepOnce(t *testing.T) {
	// release -> [test, lint]; test -> [build]; lint -> [build].
	deps := map[string][]Ref{
		"build": {},
		"test":  {{Kind: KindCommand, Name: "build"}},
		"lint":  {{Kind: KindCommand, Name: "build"}},
	}
	var buildCount atomic.Int32
	runner := func(_ context.Context, ref Ref) error {
		if ref.Name == "build" {
			buildCount.Add(1)
		}
		return nil
	}

	direct := []Ref{
		{Kind: KindCommand, Name: "test"},
		{Kind: KindCommand, Name: "lint"},
	}
	err := Run(
		context.Background(), direct,
		WithCommandRunner(runner),
		WithCommandLookup(staticLookup(deps)),
	)
	require.NoError(t, err)
	assert.Equal(t, int32(1), buildCount.Load(), "build must execute exactly once despite two dependents")
}

func TestRun_DifferentParametersBothExecute(t *testing.T) {
	runner, ran := countingRunner(t)
	direct := []Ref{
		{Kind: KindCommand, Name: "build", Flags: map[string]string{"env": "dev"}},
		{Kind: KindCommand, Name: "build", Flags: map[string]string{"env": "prod"}},
	}
	err := Run(
		context.Background(), direct,
		WithCommandRunner(runner),
		WithCommandLookup(staticLookup(map[string][]Ref{"build": {}})),
	)
	require.NoError(t, err)
	assert.Len(t, ran(), 2, "differently-parameterized invocations of the same command must both run")
}

func TestRun_SameParametersDedup(t *testing.T) {
	runner, ran := countingRunner(t)
	direct := []Ref{
		{Kind: KindCommand, Name: "build", Flags: map[string]string{"env": "dev"}},
		{Kind: KindCommand, Name: "build", Flags: map[string]string{"env": "dev"}},
	}
	err := Run(
		context.Background(), direct,
		WithCommandRunner(runner),
		WithCommandLookup(staticLookup(map[string][]Ref{"build": {}})),
	)
	require.NoError(t, err)
	assert.Len(t, ran(), 1, "identical name+params must collapse into a single executed node")
}

func TestRun_UnknownReferenceErrors(t *testing.T) {
	runner, _ := countingRunner(t)
	direct := []Ref{{Kind: KindCommand, Name: "bogus"}}
	err := Run(
		context.Background(), direct,
		WithCommandRunner(runner),
		WithCommandLookup(staticLookup(map[string][]Ref{})),
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownDependency)
}

func TestRun_CycleErrors(t *testing.T) {
	// a -> b -> a.
	deps := map[string][]Ref{
		"a": {{Kind: KindCommand, Name: "b"}},
		"b": {{Kind: KindCommand, Name: "a"}},
	}
	runner, _ := countingRunner(t)
	direct := []Ref{{Kind: KindCommand, Name: "a"}}
	err := Run(
		context.Background(), direct,
		WithCommandRunner(runner),
		WithCommandLookup(staticLookup(deps)),
	)
	require.Error(t, err)
}

func TestRun_FailurePropagatesByDefault(t *testing.T) {
	failingRunner := func(_ context.Context, ref Ref) error {
		if ref.Name == "build" {
			return assert.AnError
		}
		return nil
	}
	direct := []Ref{{Kind: KindCommand, Name: "build"}}
	err := Run(
		context.Background(), direct,
		WithCommandRunner(failingRunner),
		WithCommandLookup(staticLookup(map[string][]Ref{"build": {}})),
	)
	require.Error(t, err, "wait_all (default) must still surface the failure at the end")
}

func TestRun_BestEffortSwallowsFailure(t *testing.T) {
	failingRunner := func(_ context.Context, ref Ref) error {
		if ref.Name == "build" {
			return assert.AnError
		}
		return nil
	}
	direct := []Ref{{Kind: KindCommand, Name: "build", Fail: FailBestEffort}}
	err := Run(
		context.Background(), direct,
		WithCommandRunner(failingRunner),
		WithCommandLookup(staticLookup(map[string][]Ref{"build": {}})),
	)
	require.NoError(t, err, "best_effort must not surface the failure")
}

// TestRun_BestEffortLogsSwallowedFailure guards against best_effort's forgiven failure being
// silently invisible: Run must still log the aggregate error at warn level before returning nil.
func TestRun_BestEffortLogsSwallowedFailure(t *testing.T) {
	originalLogger := log.Default()
	defer log.SetDefault(originalLogger)

	var buf bytes.Buffer
	testLogger := log.New()
	testLogger.SetOutput(&buf)
	testLogger.SetLevel(log.WarnLevel)
	log.SetDefault(testLogger)

	failingRunner := func(_ context.Context, ref Ref) error {
		if ref.Name == "build" {
			return assert.AnError
		}
		return nil
	}
	direct := []Ref{{Kind: KindCommand, Name: "build", Fail: FailBestEffort}}
	err := Run(
		context.Background(), direct,
		WithCommandRunner(failingRunner),
		WithCommandLookup(staticLookup(map[string][]Ref{"build": {}})),
	)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Dependency failures ignored due to fail: best_effort")
}

func TestRun_WorkflowKindUsesWorkflowRunnerAndLookup(t *testing.T) {
	runner, ran := countingRunner(t)
	direct := []Ref{{Kind: KindWorkflow, Name: "deploy", File: "deploy.yaml"}}
	err := Run(
		context.Background(), direct,
		WithWorkflowRunner(runner),
		WithWorkflowLookup(staticLookup(map[string][]Ref{"deploy": {}})),
	)
	require.NoError(t, err)
	require.Len(t, ran(), 1)
	assert.Equal(t, KindWorkflow, ran()[0].Kind)
	assert.Equal(t, "deploy.yaml", ran()[0].File)
}

func TestRun_MissingRunnerErrors(t *testing.T) {
	direct := []Ref{{Kind: KindCommand, Name: "build"}}
	err := Run(
		context.Background(), direct,
		WithCommandLookup(staticLookup(map[string][]Ref{"build": {}})),
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMissingRunner)
}

func TestRun_MissingLookupErrors(t *testing.T) {
	runner, _ := countingRunner(t)
	direct := []Ref{{Kind: KindCommand, Name: "build"}}
	err := Run(context.Background(), direct, WithCommandRunner(runner))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMissingLookup)
}

func TestRun_EmptyDirectIsNoop(t *testing.T) {
	err := Run(context.Background(), nil)
	require.NoError(t, err)
}

func TestRefNodeID_StableAcrossFlagOrder(t *testing.T) {
	a := Ref{Kind: KindCommand, Name: "build", Flags: map[string]string{"env": "dev", "region": "us-east-1"}}
	b := Ref{Kind: KindCommand, Name: "build", Flags: map[string]string{"region": "us-east-1", "env": "dev"}}
	assert.Equal(t, a.NodeID(), b.NodeID(), "map iteration order must not affect NodeID")
}

func TestRefNodeID_DiffersOnFile(t *testing.T) {
	a := Ref{Kind: KindWorkflow, Name: "test", File: "a.yaml"}
	b := Ref{Kind: KindWorkflow, Name: "test", File: "b.yaml"}
	assert.NotEqual(t, a.NodeID(), b.NodeID())
}

// TestRun_MaxConcurrencyAllowsBoundedParallelism proves WithMaxConcurrency's plumbing into the
// scheduler actually has an effect: two independent nodes must be able to run concurrently, up
// to the configured bound, rather than the scheduler's own baseline of one worker. A barrier
// (both dispatches must arrive before either is released) makes this deterministic instead of
// timing-dependent -- if the bound were not actually applied, this test would time out and fail
// rather than pass "by accident".
func TestRun_MaxConcurrencyAllowsBoundedParallelism(t *testing.T) {
	const want = 2
	arrived := make(chan struct{}, want)
	release := make(chan struct{})
	//nolint:unparam // must match the Runner type signature; this test only exercises concurrency, not error propagation.
	runner := func(_ context.Context, _ Ref) error {
		arrived <- struct{}{}
		select {
		case <-release:
		case <-time.After(2 * time.Second):
		}
		return nil
	}

	direct := []Ref{
		{Kind: KindCommand, Name: "a"},
		{Kind: KindCommand, Name: "b"},
	}
	done := make(chan error, 1)
	go func() {
		done <- Run(
			context.Background(), direct,
			WithMaxConcurrency(want),
			WithCommandRunner(runner),
			WithCommandLookup(staticLookup(map[string][]Ref{"a": {}, "b": {}})),
		)
	}()

	for i := 0; i < want; i++ {
		select {
		case <-arrived:
		case <-time.After(2 * time.Second):
			t.Fatalf("expected %d concurrent dispatches within the max concurrency bound, only observed %d", want, i)
		}
	}
	close(release)
	require.NoError(t, <-done)
}

// TestRun_FailFastSkipsPendingIndependentWork proves fail_fast actually changes scheduling
// behavior versus the wait_all default: once a direct dependency declares Fail: FailFast and
// fails, still-pending INDEPENDENT siblings must be skipped rather than dispatched.
// WithMaxConcurrency(1) forces strictly sequential, alphabetically-ordered dispatch (the ready
// queue is sorted by NodeID) so "aaa-fails" is guaranteed to run before "zzz-independent" is ever
// considered, making the assertion deterministic rather than a race.
func TestRun_FailFastSkipsPendingIndependentWork(t *testing.T) {
	runner, ran := countingRunner(t)
	failingRunner := func(ctx context.Context, ref Ref) error {
		if ref.Name == "aaa-fails" {
			return assert.AnError
		}
		return runner(ctx, ref)
	}
	direct := []Ref{
		{Kind: KindCommand, Name: "aaa-fails", Fail: FailFast},
		{Kind: KindCommand, Name: "zzz-independent"},
	}
	err := Run(
		context.Background(), direct,
		WithMaxConcurrency(1),
		WithCommandRunner(failingRunner),
		WithCommandLookup(staticLookup(map[string][]Ref{"aaa-fails": {}, "zzz-independent": {}})),
	)
	require.Error(t, err)
	assert.Empty(t, ran(), "fail_fast must skip independent pending work after the first failure, unlike wait_all")
}

// TestRun_UnknownKindErrors covers the Kind-validation branch in lookupAndRunnerFor (reached
// both directly from graphVisitor.visit and, defensively, from the scheduler dispatcher).
func TestRun_UnknownKindErrors(t *testing.T) {
	runner, _ := countingRunner(t)
	direct := []Ref{{Kind: "bogus-kind", Name: "x"}}
	err := Run(
		context.Background(), direct,
		WithCommandRunner(runner),
		WithCommandLookup(staticLookup(map[string][]Ref{})),
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownDependencyKind)
}

// TestRun_LookupCallbackErrorPropagates covers the case where a Lookup callback itself returns
// an error (distinct from found=false), which must propagate out of Run rather than being
// swallowed.
func TestRun_LookupCallbackErrorPropagates(t *testing.T) {
	runner, _ := countingRunner(t)
	failingLookup := func(_ Ref) ([]Ref, bool, error) {
		return nil, false, assert.AnError
	}
	direct := []Ref{{Kind: KindCommand, Name: "build"}}
	err := Run(
		context.Background(), direct,
		WithCommandRunner(runner),
		WithCommandLookup(failingLookup),
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

// TestRun_NestedUnknownDependencyErrors covers visitChildren's error-propagation branch for a
// NESTED (non-direct) unknown reference -- distinct from TestRun_UnknownReferenceErrors, whose
// "bogus" ref is itself a direct entry and never goes through visitChildren's error path.
func TestRun_NestedUnknownDependencyErrors(t *testing.T) {
	deps := map[string][]Ref{
		"a": {{Kind: KindCommand, Name: "missing"}},
	}
	runner, _ := countingRunner(t)
	direct := []Ref{{Kind: KindCommand, Name: "a"}}
	err := Run(
		context.Background(), direct,
		WithCommandRunner(runner),
		WithCommandLookup(staticLookup(deps)),
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownDependency)
}

// TestRun_SelfDependencyErrors covers visitChildren's AddDependency-error propagation branch: a
// command whose own lookup declares itself as a dependency collapses (via NodeID dedup) into an
// attempt to add a self-referencing graph edge, which pkg/dependency correctly rejects.
func TestRun_SelfDependencyErrors(t *testing.T) {
	deps := map[string][]Ref{
		"a": {{Kind: KindCommand, Name: "a"}},
	}
	runner, _ := countingRunner(t)
	direct := []Ref{{Kind: KindCommand, Name: "a"}}
	err := Run(
		context.Background(), direct,
		WithCommandRunner(runner),
		WithCommandLookup(staticLookup(deps)),
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, dependency.ErrSelfDependency)
}

// TestGraphVisitor_Visit_PropagatesAddNodeError covers visit()'s AddNode-error propagation
// branch. Unreachable through the public Run() API under normal operation (the visited-set dedup
// guarantees AddNode is only ever called once per unique NodeID within a single Run), so this
// constructs the scenario directly at the graphVisitor level: a builder pre-populated with a
// node sharing the Ref's NodeID, but a visitor that has not yet marked it visited. This asserts
// the real contract that visit() must surface (not silently drop) a builder-level failure.
func TestGraphVisitor_Visit_PropagatesAddNodeError(t *testing.T) {
	ref := Ref{Kind: KindCommand, Name: "build"}
	builder := dependency.NewBuilder()
	require.NoError(t, builder.AddNode(&dependency.Node{ID: ref.NodeID()}))

	v := &graphVisitor{
		builder: builder,
		visited: make(map[string]struct{}),
		options: &Options{
			commandLookup: staticLookup(map[string][]Ref{"build": {}}),
			commandRunner: func(context.Context, Ref) error { return nil },
		},
	}

	err := v.visit(&ref)
	require.Error(t, err)
	assert.ErrorIs(t, err, dependency.ErrAddNodeFailed)
}

// TestNewDispatcher_DefensiveChecks covers the scheduler-dispatcher's own defensive validation,
// exercised directly since none of these branches are reachable through the public Run() API:
// graphVisitor.visit already validates Kind and Runner/Lookup presence before a node is ever
// added to the graph, so a graph built via buildGraph can never reach the dispatcher with
// missing ref metadata, an unknown Kind, or a nil runner. These checks still guard against a
// differently-constructed *dependency.Graph being fed to the same dispatcher.
func TestNewDispatcher_DefensiveChecks(t *testing.T) {
	t.Run("missing ref metadata", func(t *testing.T) {
		dispatcher := newDispatcher(&Options{})
		node := &dependency.Node{ID: "x", Metadata: map[string]any{}}
		_, err := dispatcher.Dispatch(context.Background(), node)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnknownDependencyKind)
	})

	t.Run("unknown kind", func(t *testing.T) {
		ref := Ref{Kind: "bogus-kind", Name: "x"}
		dispatcher := newDispatcher(&Options{})
		node := &dependency.Node{ID: ref.NodeID(), Metadata: map[string]any{"ref": ref}}
		_, err := dispatcher.Dispatch(context.Background(), node)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnknownDependencyKind)
	})

	t.Run("missing runner", func(t *testing.T) {
		ref := Ref{Kind: KindCommand, Name: "build"}
		dispatcher := newDispatcher(&Options{}) // No commandRunner configured.
		node := &dependency.Node{ID: ref.NodeID(), Metadata: map[string]any{"ref": ref}}
		_, err := dispatcher.Dispatch(context.Background(), node)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrMissingRunner)
	})
}
