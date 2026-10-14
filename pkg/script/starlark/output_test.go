package starlark

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
)

func executeWithStreams(t *testing.T, source string, opts ...Option) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	_, err = New(opts...).Execute(context.Background(), script.Spec{
		Name: "test.star", Source: source, Stdout: &out, Stderr: &errOut,
	})
	return out.String(), errOut.String(), err
}

func TestParallelTaskOutputIsLineAtomicAndPrefixed(t *testing.T) {
	t.Parallel()
	const tasks, lines = 8, 150
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		// Deliver each line in two chunks, as a real pipe may, from competing goroutines.
		for i := range lines {
			_, _ = io.WriteString(spec.Streams.Stdout, fmt.Sprintf("%s proc ", spec.Args[0]))
			_, _ = io.WriteString(spec.Streams.Stdout, fmt.Sprintf("line %d\n", i))
			_, _ = io.WriteString(spec.Streams.Stderr, fmt.Sprintf("%s err line %d\n", spec.Args[0], i))
		}
		return process.Result{Started: true}
	})
	stdout, stderr, err := executeWithStreams(t, fmt.Sprintf(`
def work(name):
    for i in range(%d):
        print("%%s print line %%d" %% (name, i))
    exec.run(["tool", name])
    print("%%s partial end" %% name)
steps.parallel(
    tasks = [steps.task(name = "t%%d" %% n, function = work, args = ["t%%d" %% n]) for n in range(%d)],
    max_concurrency = %d,
)
`, lines, tasks, tasks), WithProcessRunner(runner))
	require.NoError(t, err)

	counts := map[string]int{}
	pattern := regexp.MustCompile(`^\[(t\d)\] (t\d) (print line \d+|proc line \d+|partial end)$`)
	for _, line := range strings.Split(strings.TrimSuffix(stdout, "\n"), "\n") {
		match := pattern.FindStringSubmatch(line)
		require.NotNil(t, match, "torn or unprefixed stdout line: %q", line)
		assert.Equal(t, match[1], match[2], "line attributed to the wrong task: %q", line)
		counts[match[1]]++
	}
	assert.Len(t, counts, tasks)
	for name, count := range counts {
		assert.Equal(t, 2*lines+1, count, name)
	}
	errPattern := regexp.MustCompile(`^\[(t\d)\] (t\d) err line \d+$`)
	errLines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	assert.Len(t, errLines, tasks*lines)
	for _, line := range errLines {
		match := errPattern.FindStringSubmatch(line)
		require.NotNil(t, match, "torn or unprefixed stderr line: %q", line)
		assert.Equal(t, match[1], match[2])
	}
}

func TestMainThreadAndSingleTaskOutputIsUnprefixed(t *testing.T) {
	t.Parallel()
	stdout, _, err := executeWithStreams(t, `
print("main before")
def only():
    print("from the only task")
steps.parallel(functions = [only])
print("main after")
`)
	require.NoError(t, err)
	assert.Equal(t, "main before\nfrom the only task\nmain after\n", stdout)
}

func TestTaskPartialLineIsFlushedWhenTaskEnds(t *testing.T) {
	t.Parallel()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		_, _ = io.WriteString(spec.Streams.Stdout, "no newline "+spec.Args[0])
		return process.Result{Started: true}
	})
	stdout, _, err := executeWithStreams(t, `
def run(name):
    exec.run(["tool", name])
steps.parallel(tasks = [steps.task(name = n, function = run, args = [n]) for n in ["a", "b"]], max_concurrency = 1)
`, WithProcessRunner(runner))
	require.NoError(t, err)
	assert.Equal(t, "[a] no newline a\n[b] no newline b\n", stdout)
}

func TestNestedParallelPrefixesCompose(t *testing.T) {
	t.Parallel()
	stdout, _, err := executeWithStreams(t, `
def inner():
    print("leaf")
def outer(name):
    steps.parallel(tasks = [steps.task(name = "x", function = inner), steps.task(name = "y", function = inner)], max_concurrency = 1)
steps.parallel(tasks = [steps.task(name = "a", function = outer, args = ["a"]), steps.task(name = "b", function = outer, args = ["b"])], max_concurrency = 1)
`)
	require.NoError(t, err)
	assert.Equal(t, "[a] [x] leaf\n[a] [y] leaf\n[b] [x] leaf\n[b] [y] leaf\n", stdout)
}

func TestCaptureOutputForwardsNothing(t *testing.T) {
	t.Parallel()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		_, _ = io.WriteString(spec.Streams.Stdout, "captured\n")
		_, _ = io.WriteString(spec.Streams.Stderr, "diagnostic\n")
		return process.Result{Started: true}
	})
	stdout, stderr, err := executeWithStreams(t, `
r = exec.run(["tool"], output = "capture")
print(r.stdout.strip(), r.stderr.strip())
`, WithProcessRunner(runner))
	require.NoError(t, err)
	assert.Equal(t, "captured diagnostic\n", stdout)
	assert.Empty(t, stderr)
}
