package starlark

//go:generate go run go.uber.org/mock/mockgen -destination mock_process_test.go -package starlark github.com/cloudposse/atmos/pkg/process Runner
//go:generate go run go.uber.org/mock/mockgen -destination mock_clock_test.go -package starlark github.com/cloudposse/atmos/pkg/retry Clock

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
)

func runSource(t *testing.T, source string, opts ...Option) (script.Result, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts = append([]Option{WithAtmosCommands(testAtmosCatalog())}, opts...)
	return New(opts...).Execute(ctx, script.Spec{Name: "test.star", Source: source})
}

func TestEngineOutput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source, value string
		hasOutput           bool
	}{
		{"structured", `output = {"values": [1, True, None]}`, `{"values":[1,true,null]}`, true},
		{"string is raw", `output = "hello"`, `hello`, true},
		{"empty string is raw", `output = ""`, ``, true},
		{"encoded string", `output = json.encode("hello")`, `"hello"`, true},
		{"none", `output = None`, `null`, true},
		{"print only", `print("hello")`, ``, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := runSource(t, tc.source)
			require.NoError(t, err)
			assert.Equal(t, tc.value, result.Value)
			assert.Equal(t, tc.hasOutput, result.HasOutput)
		})
	}
}

func TestParallelFunctionsAndLoadedModules(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "lib"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "lib", "value.star"), []byte("value = 42"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "lib", "deploy.star"), []byte(`load("value.star", "value")
def deploy():
    return value
`), 0o600))
	result, err := New().Execute(context.Background(), script.Spec{WorkingDirectory: dir, Source: `
load("lib/deploy.star", "deploy")
def other():
    return "other"
output = steps.parallel(functions=[deploy, other])
`})
	require.NoError(t, err)
	assert.JSONEq(t, `[42,"other"]`, result.Value)
}

func TestParallelBoundedConcurrencyAndResultOrder(t *testing.T) {
	t.Parallel()
	runner := NewMockRunner(gomock.NewController(t))
	var active, peak atomic.Int32
	arrived := make(chan string, 3)
	release := map[string]chan struct{}{"first": make(chan struct{}), "second": make(chan struct{}), "third": make(chan struct{})}
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(3).DoAndReturn(func(ctx context.Context, spec process.TaskSpec) process.Result {
		current := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); current > old && !peak.CompareAndSwap(old, current); old = peak.Load() {
		}
		arrived <- spec.Command
		select {
		case <-release[spec.Command]:
			_, _ = io.WriteString(spec.Streams.Stdout, spec.Command)
			return process.Result{}
		case <-ctx.Done():
			return process.Result{Err: ctx.Err()}
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type outcome struct {
		result script.Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := New(WithProcessRunner(runner)).Execute(ctx, script.Spec{Source: `
def run(name):
    return exec.run([name]).stdout
output = steps.parallel(tasks=[steps.task(name=n, function=run, args=[n]) for n in ["first", "second", "third"]], max_concurrency=2)
`})
		done <- outcome{result, err}
	}()
	for range 2 {
		select {
		case <-arrived:
		case <-ctx.Done():
			t.Fatal("branches did not overlap")
		}
	}
	assert.Equal(t, int32(2), active.Load())
	close(release["second"])
	select {
	case name := <-arrived:
		assert.Equal(t, "third", name)
	case <-ctx.Done():
		t.Fatal("third branch did not start")
	}
	close(release["third"])
	close(release["first"])
	out := <-done
	require.NoError(t, out.err)
	assert.JSONEq(t, `["first","second","third"]`, out.result.Value)
	assert.Equal(t, int32(2), peak.Load())
	assert.Zero(t, active.Load(), "join must wait for every branch")
}

func TestParallelRetryWithFakeClock(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	runner, clock := NewMockRunner(ctrl), NewMockClock(ctrl)
	clock.EXPECT().Now().AnyTimes().Return(time.Unix(0, 0))
	clock.EXPECT().After(2 * time.Second).Times(2).DoAndReturn(func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Unix(2, 0)
		return ch
	})
	var attempts sync.Map
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(4).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		counter, _ := attempts.LoadOrStore(spec.Command, &atomic.Int32{})
		n := counter.(*atomic.Int32).Add(1)
		if spec.Command == "flaky" && n < 3 {
			return process.Result{Err: errUtils.ErrProcessWaitFailed, ExitCode: 1}
		}
		return process.Result{}
	})
	var events []TaskEvent
	result, err := runSource(t, `
def deploy(name, suffix):
    exec.run([name])
    return name + suffix
output = steps.parallel(tasks=[
    steps.task(name=n, function=deploy, args=[n], kwargs={"suffix": "-ok"}, retry={"max_attempts": 3, "initial_delay": "2s"})
    for n in ["flaky", "steady"]
])`, WithProcessRunner(runner), WithRetryClock(clock), WithTaskObserver(func(event TaskEvent) { events = append(events, event) }))
	require.NoError(t, err)
	assert.JSONEq(t, `["flaky-ok","steady-ok"]`, result.Value)
	for name, want := range map[string]int32{"flaky": 3, "steady": 1} {
		counter, ok := attempts.Load(name)
		require.True(t, ok)
		assert.Equal(t, want, counter.(*atomic.Int32).Load())
	}
	assert.Contains(t, events, TaskEvent{Name: "flaky", Status: "attempt", Attempt: 3})
	assert.Contains(t, events, TaskEvent{Name: "steady", Status: "succeeded"})
}

func TestParallelWaitAllAndFailFast(t *testing.T) {
	t.Parallel()
	for _, failFast := range []bool{false, true} {
		t.Run(fmt.Sprint(failFast), func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			var completed atomic.Bool
			started := make(chan struct{})
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(func(ctx context.Context, spec process.TaskSpec) process.Result {
				if spec.Command == "bad" {
					select {
					case <-started:
					case <-ctx.Done():
						return process.Result{Err: ctx.Err()}
					}
					return process.Result{Err: errUtils.ErrProcessWaitFailed, ExitCode: 1}
				}
				close(started)
				if failFast {
					<-ctx.Done()
					return process.Result{Err: ctx.Err()}
				}
				completed.Store(true)
				return process.Result{}
			})
			_, err := runSource(t, fmt.Sprintf(`
def run(name):
    return exec.run([name]).exit_code
steps.parallel(tasks=[steps.task(name=n, function=run, args=[n]) for n in ["bad", "good"]], fail_fast=%s)
`, map[bool]string{false: "False", true: "True"}[failFast]), WithProcessRunner(runner))
			require.ErrorIs(t, err, errUtils.ErrStarlark)
			assert.Contains(t, err.Error(), `task "bad"`)
			assert.Equal(t, !failFast, completed.Load())
		})
	}
}

func TestParallelValidation(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`steps.parallel()`, `steps.parallel(functions=[], tasks=[])`,
		`steps.parallel(functions=[], max_concurrency=0)`, `steps.parallel(functions=[1])`,
		`steps.parallel(tasks=[lambda: 1])`, `steps.parallel(functions="bad")`,
		`steps.task(name="", function=lambda: 1)`,
		`steps.task(name="bad", function=lambda: 1, timeout="0s")`,
		`steps.task(name="bad", function=lambda: 1, timeout="bogus")`,
		`steps.task(name="bad", function=lambda: 1, args="bad")`,
		`steps.task(name="bad", function=lambda: 1, kwargs={1: 2})`,
		`steps.task(name="bad", function=lambda: 1, retry={"max_attempts": 0})`,
		`steps.task(name="bad", function=lambda: 1, retry={"unknown": 1})`,
		`steps.task(name="bad", function=lambda: 1, retry={"backoff_strategy": "bogus"})`,
		`steps.parallel(tasks=[steps.task(name="same", function=lambda: 1)] * 2)`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			_, err := runSource(t, source)
			require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
		})
	}
}

func TestEngineIsolationAndDryRun(t *testing.T) {
	t.Parallel()
	runner := NewMockRunner(gomock.NewController(t))
	_, err := New(WithProcessRunner(runner)).Execute(context.Background(), script.Spec{DryRun: true, Source: `exec.run(["never"])`})
	require.NoError(t, err)
	var stdout bytes.Buffer
	result, err := New().Execute(context.Background(), script.Spec{Env: map[string]string{"EXPLICIT": "yes"}, ProcessEnv: []string{"AMBIENT=hidden"}, Stdout: &stdout, Source: `
print("hello")
output = env
`})
	require.NoError(t, err)
	assert.JSONEq(t, `{"EXPLICIT":"yes"}`, result.Value)
	assert.Equal(t, "hello\n", stdout.String())
}

func TestLoadCycle(t *testing.T) {
	t.Parallel()
	_, err := runSource(t, `load("cycle.star", "value")`, WithReadFile(func(string) ([]byte, error) {
		return []byte(`load("cycle.star", "value")`), nil
	}))
	require.ErrorContains(t, err, "cyclic load")
}

func TestParallelFreezesGlobalsClosuresAndArguments(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`shared = []
def change():
    shared.append(1)
steps.parallel(functions=[change, change])`,
		`def make():
    values = []
    def change():
        values.append(1)
    return change
steps.parallel(functions=[make()])`,
		`def change(values):
    values["nested"].append(1)
steps.parallel(tasks=[steps.task(name="change", function=change, args=[{"nested": []}])])`,
	} {
		t.Run(strings.Split(source, "\n")[0], func(t *testing.T) {
			t.Parallel()
			_, err := runSource(t, source)
			require.ErrorContains(t, err, "frozen")
		})
	}
	result, err := runSource(t, `
def local():
    values = []
    values.append(1)
    return values
output = steps.parallel(functions=[local, local])`)
	require.NoError(t, err)
	assert.JSONEq(t, `[[1],[1]]`, result.Value)
}

func TestTaskTimeoutCancelsEvaluation(t *testing.T) {
	t.Parallel()
	_, err := runSource(t, `
def busy():
    for i in range(1000000000):
        pass
steps.parallel(tasks=[steps.task(name="busy", function=busy, timeout="5ms")])`)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestParallelNestedAndEmpty(t *testing.T) {
	t.Parallel()
	result, err := runSource(t, `
def leaf():
    return 42
def branch():
    return steps.parallel(functions=[leaf, leaf], max_concurrency=1)
output = [steps.parallel(functions=[branch, branch]), steps.parallel(tasks=[])]`)
	require.NoError(t, err)
	assert.JSONEq(t, `[[[42,42],[42,42]],[]]`, result.Value)
}
