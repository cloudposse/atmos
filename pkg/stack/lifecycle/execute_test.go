package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/scheduler"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type runnerFunc func(context.Context, process.TaskSpec) process.Result

//nolint:gocritic // Matches the process.Runner interface.
func (f runnerFunc) Run(ctx context.Context, s process.TaskSpec) process.Result { return f(ctx, s) }

func executionPlan(t *testing.T) *Plan {
	t.Helper()
	a := []string{"terraform", "dev", "a"}
	b := []string{"helm", "dev", "b"}
	c := []string{"kubernetes", "dev", "c"}
	stacks := inventory(a, b, c)
	depends(stacks, c, a, true)
	p, err := BuildPlan(stacks, capabilities("terraform", "helm", "kubernetes"), Selection{}, "{{")
	require.NoError(t, err)
	return p
}

func TestExecutionConcurrencyReadinessAndArguments(t *testing.T) {
	p := executionPlan(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := make(chan string, 3)
	release := make(chan struct{})
	finished := make(chan *scheduler.AggregateResult, 1)
	var mu sync.Mutex
	active, maxActive := 0, 0
	done := map[string]bool{}
	var violations []string
	runner := runnerFunc(func(ctx context.Context, s process.TaskSpec) process.Result {
		name := s.Args[len(s.Args)-1]
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		if name == "c" && !done["a"] {
			violations = append(violations, "dependent started before prerequisite finished")
		}
		mu.Unlock()
		if s.Streams.Stdin != nil {
			mu.Lock()
			violations = append(violations, "concurrent stdin")
			mu.Unlock()
		}
		if s.Args[0] != "--identity=test" {
			mu.Lock()
			violations = append(violations, "lost identity")
			mu.Unlock()
		}
		started <- name
		select {
		case <-release:
		case <-ctx.Done():
			return process.Result{ExitCode: 1, Err: ctx.Err()}
		}
		fmt.Fprintln(s.Streams.Stdout, "output "+name)
		mu.Lock()
		active--
		done[name] = true
		mu.Unlock()
		return process.Result{}
	})
	var logs bytes.Buffer
	go func() {
		finished <- Execute(ctx, p, &ExecutionOptions{Executable: "fake-atmos", GlobalArgs: []string{"--identity=test"}, Runner: runner, Concurrency: 2, Streams: process.Streams{Stdin: strings.NewReader("yes"), Stdout: &logs, Stderr: &logs}})
	}()
	first := []string{}
	for range 2 {
		select {
		case n := <-started:
			first = append(first, n)
		case <-ctx.Done():
			t.Fatal("independent nodes did not overlap")
		}
	}
	assert.ElementsMatch(t, []string{"a", "b"}, first)
	close(release)
	var result *scheduler.AggregateResult
	select {
	case result = <-finished:
	case <-ctx.Done():
		t.Fatal("scheduler did not finish")
	}
	require.NoError(t, result.Err)
	assert.Equal(t, 2, maxActive)
	assert.Empty(t, violations)
	assert.Contains(t, logs.String(), "[terraform/dev/a] output a")
}

func TestExecutionFailureModesAndCancellation(t *testing.T) {
	for _, keep := range []bool{false, true} {
		t.Run(fmt.Sprint(keep), func(t *testing.T) {
			p := executionPlan(t)
			// Assign the first sorted root to fail, regardless of component kind.
			order, err := p.Graph.TopologicalSort()
			require.NoError(t, err)
			failed := order[0].Component
			runner := runnerFunc(func(_ context.Context, s process.TaskSpec) process.Result {
				if s.Args[len(s.Args)-1] == failed {
					return process.Result{ExitCode: 7}
				}
				return process.Result{}
			})
			result := Execute(context.Background(), p, &ExecutionOptions{Runner: runner, Concurrency: 1, KeepGoing: keep, Streams: process.Streams{Stderr: io.Discard}})
			require.Error(t, result.Err)
			counts := map[scheduler.Status]int{}
			for _, r := range result.Results {
				counts[r.Status]++
			}
			assert.Equal(t, 1, counts[scheduler.StatusFailed])
			if !keep {
				assert.Positive(t, counts[scheduler.StatusSkipped])
			} else {
				assert.Positive(t, counts[scheduler.StatusSucceeded])
			}
		})
	}
	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		started := make(chan struct{}, 1)
		runner := runnerFunc(func(ctx context.Context, _ process.TaskSpec) process.Result {
			started <- struct{}{}
			<-ctx.Done()
			return process.Result{ExitCode: 1, Err: ctx.Err()}
		})
		finished := make(chan *scheduler.AggregateResult, 1)
		go func() { finished <- Execute(ctx, executionPlan(t), &ExecutionOptions{Runner: runner, Concurrency: 1}) }()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("no start")
		}
		cancel()
		select {
		case r := <-finished:
			require.Error(t, r.Err)
		case <-time.After(5 * time.Second):
			t.Fatal("cancel did not return")
		}
	})
}

func TestSingleWorkerPreservesInteractiveStreams(t *testing.T) {
	input := strings.NewReader("yes\n")
	var output bytes.Buffer
	runner := runnerFunc(func(_ context.Context, s process.TaskSpec) process.Result {
		assert.Same(t, input, s.Streams.Stdin)
		assert.Same(t, &output, s.Streams.Stdout)
		return process.Result{}
	})
	result := Execute(context.Background(), executionPlan(t), &ExecutionOptions{Runner: runner, Concurrency: 1, Grouped: true, Streams: process.Streams{Stdin: input, Stdout: &output, Stderr: &output}})
	require.NoError(t, result.Err)
}

func TestChildEnvironment(t *testing.T) {
	base := []string{"ATMOS_ALL=true", "ATMOS_TERRAFORM_DESTROY_AFFECTED=true", "ATMOS_INCLUDE_DEPENDENCIES=-1", "ATMOS_PROFILE=dev", "TF_PLUGIN_CACHE_DIR=/shared"}
	env := childEnvironment(base)
	assert.NotContains(t, env, "ATMOS_ALL=true")
	assert.NotContains(t, env, "ATMOS_TERRAFORM_DESTROY_AFFECTED=true")
	assert.Contains(t, env, "ATMOS_PROFILE=dev")
	assert.Contains(t, env, "TF_PLUGIN_CACHE_DIR=/shared")
}

func TestChildEnvironmentStripsDryRun(t *testing.T) {
	env := childEnvironment([]string{"ATMOS_DRY_RUN=true", "ATMOS_PROFILE=dev"})
	assert.NotContains(t, env, "ATMOS_DRY_RUN=true")
	assert.Equal(t, []string{"ATMOS_PROFILE=dev"}, env)
	// The scrubbed environment is what the runner receives.
	var seen []string
	runner := runnerFunc(func(_ context.Context, s process.TaskSpec) process.Result {
		seen = s.Env
		return process.Result{}
	})
	result := Execute(context.Background(), executionPlan(t), &ExecutionOptions{Runner: runner, Concurrency: 1, Environment: []string{"ATMOS_DRY_RUN=true", "KEEP=1"}})
	require.NoError(t, result.Err)
	assert.Equal(t, []string{"KEEP=1"}, seen)
}

func TestDispatchFailureErrorOmitsExitStatusWrapper(t *testing.T) {
	wrapper := fmt.Errorf("%w: exit status 1", errUtils.ErrProcessWaitFailed)
	for _, tc := range []struct {
		name        string
		result      process.Result
		wantWrapper bool
	}{
		{"plain non-zero exit", process.Result{Started: true, ExitCode: 1, Err: wrapper}, false},
		{"start failure", process.Result{ExitCode: -1, Err: fmt.Errorf("%w: not found", errUtils.ErrProcessStartFailed)}, true},
		{"canceled", process.Result{Started: true, ExitCode: 1, Canceled: true, Err: errors.Join(wrapper, context.Canceled)}, true},
		{"signaled", process.Result{Started: true, ExitCode: 137, Signaled: true, Signal: "killed", Err: wrapper}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := runnerFunc(func(context.Context, process.TaskSpec) process.Result { return tc.result })
			p := executionPlan(t)
			result := Execute(context.Background(), p, &ExecutionOptions{Runner: runner, Concurrency: 1})
			require.Error(t, result.Err)
			require.ErrorIs(t, result.Err, errUtils.ErrComponentExecutionFailed)
			assert.Contains(t, result.Err.Error(), "(exit ")
			if tc.wantWrapper {
				assert.Contains(t, result.Err.Error(), tc.result.Err.Error())
			} else {
				assert.NotContains(t, result.Err.Error(), "process wait failed")
				assert.NotErrorIs(t, result.Err, errUtils.ErrProcessWaitFailed)
			}
		})
	}
}

func TestGroupedOutputRoutesStreams(t *testing.T) {
	runner := runnerFunc(func(_ context.Context, s process.TaskSpec) process.Result {
		name := s.Args[len(s.Args)-1]
		fmt.Fprintln(s.Streams.Stdout, "data "+name)
		fmt.Fprintln(s.Streams.Stderr, "log "+name)
		return process.Result{}
	})
	var data, ui bytes.Buffer
	result := Execute(context.Background(), executionPlan(t), &ExecutionOptions{Runner: runner, Concurrency: 2, Grouped: true, Streams: process.Streams{Stdout: &data, Stderr: &ui}})
	require.NoError(t, result.Err)
	for _, name := range []string{"a", "b", "c"} {
		assert.Contains(t, data.String(), "data "+name+"\n")
		assert.NotContains(t, data.String(), "log "+name)
		assert.Contains(t, ui.String(), "log "+name+"\n")
		assert.NotContains(t, ui.String(), "data "+name)
	}
	// Headers belong to the UI channel only.
	assert.Contains(t, ui.String(), "[terraform/dev/a]\n")
	assert.NotContains(t, data.String(), "[terraform/dev/a]")
	// Replay is deterministic: each header precedes its component's stderr content.
	assert.Less(t, strings.Index(ui.String(), "[terraform/dev/a]"), strings.Index(ui.String(), "log a"))
}

func TestGroupedOutputToleratesMissingStreams(t *testing.T) {
	runner := runnerFunc(func(_ context.Context, s process.TaskSpec) process.Result {
		fmt.Fprintln(s.Streams.Stdout, "data")
		return process.Result{}
	})
	result := Execute(context.Background(), executionPlan(t), &ExecutionOptions{Runner: runner, Concurrency: 2, Grouped: true})
	require.NoError(t, result.Err)
}

func TestRealChildrenFreshOutputsAndGroupedLogs(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("shell fixture")
	}
	p := executionPlan(t)
	dir := t.TempDir()
	for id, n := range p.Graph.Nodes {
		script := "printf 'independent\\n'"
		switch n.Component {
		case "a":
			script = "printf new-value > output; printf 'before hook\\nafter hook\\n'"
		case "c":
			script = "test \"$(cat output)\" = new-value || exit 2; printf 'fresh output\\n'"
		}
		p.Commands[id] = []string{"-c", script}
	}
	var data, ui bytes.Buffer
	result := Execute(context.Background(), p, &ExecutionOptions{Executable: "/bin/sh", Directory: dir, Environment: os.Environ(), Runner: process.NewDefaultRunner(), Concurrency: 2, Grouped: true, Streams: process.Streams{Stdout: &data, Stderr: &ui}})
	require.NoError(t, result.Err)
	assert.Contains(t, data.String(), "before hook\nafter hook")
	assert.Contains(t, data.String(), "fresh output")
	assert.NotContains(t, ui.String(), "fresh output")
	assert.Contains(t, ui.String(), "[terraform/dev/a]")
	assert.FileExists(t, filepath.Join(dir, "output"))
}

func TestGuardEnclosesExecution(t *testing.T) {
	var held bool
	guard := func(_ *dependency.Node) func() { held = true; return func() { held = false } }
	runner := runnerFunc(func(_ context.Context, s process.TaskSpec) process.Result {
		assert.Equal(t, s.Args[0] == "terraform", held)
		return process.Result{}
	})
	r := Execute(context.Background(), executionPlan(t), &ExecutionOptions{Runner: runner, Concurrency: 1, ResourceGuards: map[string]func(*dependency.Node) func(){"terraform": guard}})
	require.NoError(t, r.Err)
	assert.False(t, held)
	bad := Execute(context.Background(), nil, &ExecutionOptions{})
	require.Error(t, bad.Err)
}
