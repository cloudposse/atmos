package taskgraph

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
