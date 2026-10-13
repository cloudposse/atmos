package starlark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
)

const prefixText = "starlark execution failed"

func TestOutputEncodeFailure(t *testing.T) {
	t.Parallel()
	_, err := runSource(t, "def f():\n    return 1\noutput = f")
	require.ErrorIs(t, err, errUtils.ErrStarlarkOutputEncode)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.Equal(t, prefixText+": `output` must be a string or JSON-encodable value: cannot encode function as JSON", err.Error())
	hints := cockroach.GetAllHints(err)
	require.Len(t, hints, 1)
	assert.Contains(t, hints[0], `"test.star"`)
}

func TestRuntimeErrorIsSingleLineWithFencedBacktrace(t *testing.T) {
	t.Parallel()
	_, err := runSource(t, "def inner(d):\n    return d[\"missing\"]\ndef outer():\n    return inner({})\nouter()")
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.Equal(t, prefixText+": key \"missing\" not in dict", err.Error())
	details := cockroach.GetAllDetails(err)
	require.Len(t, details, 1)
	assert.True(t, strings.HasPrefix(details[0], "```text\n"), details[0])
	assert.True(t, strings.HasSuffix(details[0], "\n```"), details[0])
	for _, frame := range []string{"<toplevel>", "in outer", "in inner"} {
		assert.Contains(t, details[0], frame)
	}
}

func TestProcessFailureMessages(t *testing.T) {
	t.Parallel()
	var stderr strings.Builder
	for i := 1; i <= 25; i++ {
		fmt.Fprintf(&stderr, "problem %d\n", i)
	}
	for _, tc := range []struct {
		name    string
		result  process.Result
		want    string
		details []string
		absent  []string
	}{
		{
			name:    "nonzero exit",
			result:  process.Result{Started: true, ExitCode: 3, Err: errUtils.ErrProcessWaitFailed},
			want:    prefixText + ": tool exited with code 3",
			details: []string{"problem 25", "problem 6\n"},
			absent:  []string{"problem 5\n", "problem 1\n"},
		},
		{
			name:   "start failure",
			result: process.Result{ExitCode: -1, Err: fmt.Errorf("%w: exec: \"tool\": executable file not found", errUtils.ErrProcessStartFailed)},
			want:   prefixText + `: failed to start tool: exec: "tool": executable file not found`,
			absent: []string{"code -1"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
				if tc.result.Started {
					_, _ = io.WriteString(spec.Streams.Stderr, stderr.String())
				}
				return tc.result
			})
			_, err := runSource(t, `exec.run(["tool"], output = "capture")`, WithProcessRunner(runner))
			require.ErrorIs(t, err, errUtils.ErrStarlarkProcessFailed)
			require.ErrorIs(t, err, errUtils.ErrStarlark)
			assert.Equal(t, tc.want, err.Error())
			joined := strings.Join(cockroach.GetAllDetails(err), "\n")
			for _, want := range tc.details {
				assert.Contains(t, joined, want)
			}
			for _, absent := range tc.absent {
				assert.NotContains(t, err.Error()+joined, absent)
			}
		})
	}
}

func TestProcessOutputOptionValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, source, want string }{
		{"unknown mode", `exec.run(["tool"], output = "quiet")`, `output must be "stream", "capture", or "viewport", got "quiet"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runSource(t, tc.source, WithProcessRunner(NewMockRunner(gomock.NewController(t))))
			require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestProcessCheckFalseKeepsRealExitCode(t *testing.T) {
	t.Parallel()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		_, _ = io.WriteString(spec.Streams.Stdout, "out")
		_, _ = io.WriteString(spec.Streams.Stderr, "err")
		return process.Result{Started: true, ExitCode: 42, Err: errUtils.ErrProcessWaitFailed}
	})
	result, err := runSource(t, `r = exec.run(["tool"], check = False, output = "capture")
output = [r.exit_code, r.stdout, r.stderr]`, WithProcessRunner(runner))
	require.NoError(t, err)
	assert.JSONEq(t, `[42,"out","err"]`, result.Value)
}

func TestFailedTasksYieldOneMessageEach(t *testing.T) {
	t.Parallel()
	source := `
def boom(name):
    fail("broken " + name)
steps.parallel(tasks = [steps.task(name = "a", function = boom, args = ["a"]), steps.task(name = "b", function = boom, args = ["b"])])
`
	_, err := runSource(t, source)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	message := err.Error()
	assert.Equal(t, 1, strings.Count(message, prefixText), message)
	assert.NotContains(t, message, "Traceback")
	assert.Contains(t, message, "2 tasks failed")
	assert.Contains(t, message, `task "a": fail: broken a`)
	assert.Contains(t, message, `task "b": fail: broken b`)
	joined := strings.Join(cockroach.GetAllDetails(err), "\n")
	assert.Contains(t, joined, `Traceback for task "a"`)
	assert.Contains(t, joined, `Traceback for task "b"`)
}

func TestSingleFailedTaskMessage(t *testing.T) {
	t.Parallel()
	_, err := runSource(t, `
def boom():
    fail("nope")
steps.parallel(functions = [boom])
`)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.Equal(t, prefixText+": task \"boom[0]\": fail: nope", err.Error())
}

func TestTaskTimeout(t *testing.T) {
	t.Parallel()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ process.TaskSpec) process.Result {
		<-ctx.Done()
		return process.Result{Started: true, ExitCode: -1, Canceled: true, Signaled: true, Err: ctx.Err()}
	})
	_, err := runSource(t, `
def slow():
    exec.run(["sleep"])
steps.parallel(tasks = [steps.task(name = "slow", function = slow, timeout = "50ms")])
`, WithProcessRunner(runner))
	require.ErrorIs(t, err, errUtils.ErrStarlarkTaskTimeout)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, prefixText+`: task "slow" timed out after 50ms`, err.Error())
}

func TestCanceledContextStaysReachableThroughErrors(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ process.TaskSpec) process.Result {
		cancel()
		<-ctx.Done()
		return process.Result{Started: true, ExitCode: -1, Canceled: true, Err: ctx.Err()}
	})
	_, err := New(WithProcessRunner(runner)).Execute(ctx, script.Spec{Source: `
def work():
    exec.run(["tool"])
steps.parallel(functions = [work])
`})
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	require.ErrorIs(t, err, context.Canceled)
}

func TestWorkingDirectoryMustExist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	missing := filepath.Join(dir, "missing")
	for _, tc := range []struct{ name, dir, want string }{
		{"missing", missing, fmt.Sprintf("working directory %q does not exist", missing)},
		{"file", file, fmt.Sprintf("working directory %q is not a directory", file)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			_, err := New(WithProcessRunner(runner)).Execute(t.Context(), script.Spec{
				WorkingDirectory: tc.dir, Source: `exec.run(["never"])`,
			})
			require.ErrorIs(t, err, errUtils.ErrStarlark)
			assert.Equal(t, prefixText+": "+tc.want, err.Error())
			hints := cockroach.GetAllHints(err)
			require.Len(t, hints, 1)
			assert.Contains(t, hints[0], "working_directory")
		})
	}
}

func TestPathPolicyIsConsistentForLoadAndReadFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "lib"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "work"), 0o700))
	module := filepath.Join(root, "lib", "mod.star")
	data := filepath.Join(root, "lib", "data.txt")
	require.NoError(t, os.WriteFile(module, []byte("value = 7\n"), 0o600))
	require.NoError(t, os.WriteFile(data, []byte("payload"), 0o600))

	for _, tc := range []struct{ name, loadPath, readPath string }{
		{"absolute", module, data},
		{"relative with dotdot", "../lib/mod.star", "../lib/data.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := New().Execute(t.Context(), script.Spec{
				WorkingDirectory: filepath.Join(root, "work"),
				Source:           fmt.Sprintf("load(%q, \"value\")\noutput = [value, fs.read_file(%q)]", tc.loadPath, tc.readPath),
			})
			require.NoError(t, err)
			assert.JSONEq(t, `[7,"payload"]`, result.Value)
		})
	}
}

func TestLoadCacheIsKeyedByCleanedAbsolutePathAndDetectsCycles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	files := map[string]string{
		filepath.Join(root, "lib", "mod.star"): "value = 1\n",
		filepath.Join(root, "lib", "a.star"):   "load(\"b.star\", \"b\")\na = 1\n",
		filepath.Join(root, "lib", "b.star"):   "load(\"a.star\", \"a\")\nb = 1\n",
	}
	reads := map[string]int{}
	read := func(path string) ([]byte, error) {
		reads[path]++
		contents, ok := files[path]
		if !ok {
			return nil, os.ErrNotExist
		}
		return []byte(contents), nil
	}
	module := filepath.Join(root, "lib", "mod.star")
	result, err := New(WithReadFile(read)).Execute(t.Context(), script.Spec{
		WorkingDirectory: root,
		Source:           fmt.Sprintf("load(%q, \"value\")\nload(\"lib/mod.star\", v2 = \"value\")\nload(\"lib/../lib/mod.star\", v3 = \"value\")\noutput = [value, v2, v3]", module),
	})
	require.NoError(t, err)
	assert.JSONEq(t, `[1,1,1]`, result.Value)
	assert.Equal(t, 1, reads[module], "one module evaluation regardless of path spelling")

	_, err = New(WithReadFile(read)).Execute(t.Context(), script.Spec{
		WorkingDirectory: root,
		Source:           fmt.Sprintf("load(%q, \"a\")", filepath.Join(root, "lib", "a.star")),
	})
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.Contains(t, err.Error(), "cyclic load of")
}

func TestErrorChainsKeepSingleStarlarkPrefix(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`fail("x")`,
		"def f():\n    fail(\"x\")\nsteps.parallel(functions = [f])",
		`exec.run([])`,
		`regex.search("[", "x")`,
		`fs.read_file("/definitely/not/here")`,
	} {
		_, err := runSource(t, source)
		require.ErrorIs(t, err, errUtils.ErrStarlark, source)
		assert.Equal(t, 1, strings.Count(err.Error(), prefixText), "%s: %s", source, err)
		assert.NotErrorIs(t, err, errUtils.ErrStarlarkTaskTimeout, source)
	}
}

func TestDisplayPathsRewritesOnlyPathsInsideTheRoot(t *testing.T) {
	t.Parallel()
	root := filepath.Join(string(filepath.Separator), "work", "proj")
	sep := string(filepath.Separator)
	inside := filepath.Join(root, "scripts", "boom.star")

	tests := []struct {
		name, text, want string
	}{
		{"empty text", "", ""},
		{"path at the start", inside + ":7:8: in main", filepath.Join("scripts", "boom.star") + ":7:8: in main"},
		{"indented backtrace line", "  " + inside + ":7:8: in main", "  " + filepath.Join("scripts", "boom.star") + ":7:8: in main"},
		{"inside a sentence", "cannot load " + inside + ": missing", "cannot load " + filepath.Join("scripts", "boom.star") + ": missing"},
		{"every occurrence", inside + " and " + inside, filepath.Join("scripts", "boom.star") + " and " + filepath.Join("scripts", "boom.star")},
		{"sibling directory sharing the prefix", filepath.Join(sep+"work", "proj-other", "x.star") + ":1:1", filepath.Join(sep+"work", "proj-other", "x.star") + ":1:1"},
		{"outside the root", filepath.Join(sep+"elsewhere", "x.star") + ":1:1", filepath.Join(sep+"elsewhere", "x.star") + ":1:1"},
		{"root embedded in a longer path", filepath.Join(sep+"other", "work", "proj", "x.star") + ":1:1", filepath.Join(sep+"other", "work", "proj", "x.star") + ":1:1"},
		{"the root itself", root + ": not a file", root + ": not a file"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, displayPaths(root, tc.text))
		})
	}

	t.Run("empty root is a no-op", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, inside, displayPaths("", inside))
	})

	t.Run("a root with a trailing separator behaves the same", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, filepath.Join("scripts", "boom.star"), displayPaths(root+sep, inside))
	})
}

func TestDisplayErrorKeepsTheOriginalReachable(t *testing.T) {
	t.Parallel()
	root := filepath.Join(string(filepath.Separator), "work", "proj")
	sentinel := errors.New("sentinel")
	original := fmt.Errorf("read %s: %w", filepath.Join(root, "x.star"), sentinel)

	shown := displayError(root, original)

	assert.Equal(t, "read x.star: sentinel", shown.Error())
	assert.ErrorIs(t, shown, sentinel)
	assert.Same(t, original, errors.Unwrap(shown))

	unchanged := errors.New("nothing to rewrite")
	assert.Same(t, unchanged, displayError(root, unchanged), "an error with no project path is returned as is")
	assert.NoError(t, displayError(root, nil))
}

func TestDedupeErrorLeadDropsARepeatedBuiltinName(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ in, want string }{
		"fail":           {"Error in fail: fail: boom", "Error in fail: boom"},
		"dotted builtin": {"Error in cli.command: cli.command: invalid name", "Error in cli.command: invalid name"},
		"different name": {"Error in exec.run: command failed", "Error in exec.run: command failed"},
		"no repetition":  {"Error: fail: boom", "Error: fail: boom"},
		"only the final line": {
			"Traceback (most recent call last):\n  x.star:1:1: in <toplevel>\nError in fail: fail: boom",
			"Traceback (most recent call last):\n  x.star:1:1: in <toplevel>\nError in fail: boom",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, dedupeErrorLead(tc.in))
		})
	}
}
