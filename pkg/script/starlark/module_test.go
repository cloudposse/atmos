package starlark

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/script"
)

func writeModules(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, source := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(source), 0o600))
	}
	return dir
}

func TestModuleLoadExports(t *testing.T) {
	t.Parallel()
	dir := writeModules(t, map[string]string{
		"base.star": "original = 7",
		"lib.star": `load("base.star", "original")
exported = original
VALUE = 42
_private = "private"
def _helper():
    return VALUE
def get():
    return _helper()
`,
	})
	result, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: `
lib = module.load(path="lib.star")
output = [lib.get(), lib.exported, type(lib), dir(lib), lib == module.load("./lib.star")]
`})
	require.NoError(t, err)
	assert.JSONEq(t, `[42,7,"module",["VALUE","exported","get"],true]`, result.Value)
}

func TestModuleLoadPaths(t *testing.T) {
	t.Parallel()
	dir := writeModules(t, map[string]string{
		"lib/parent.star": `eager = module.load("child.star")
def read():
    return module.load("child.star").value + eager.value
`,
		"lib/child.star":        "value = 21",
		"elsewhere/unused.star": "",
	})
	for _, tc := range []struct {
		name, source, workDir, sourcePath, want string
	}{
		{"entry file", `output = module.load("lib/parent.star").read()`, filepath.Join(dir, "elsewhere"), filepath.Join(dir, "main.star"), `42`},
		{"inline", `output = module.load("lib/parent.star").read()`, dir, "", `42`},
		{"absolute", fmt.Sprintf("output = module.load(%q).read()", filepath.Join(dir, "lib", "parent.star")), filepath.Join(dir, "elsewhere"), "", `42`},
		{"parallel function", `def run():
    return module.load("lib/parent.star").read()
output = steps.parallel(functions=[run, run])`, filepath.Join(dir, "elsewhere"), filepath.Join(dir, "main.star"), `[42,42]`},
		{"namespace functions", `lib = module.load("lib/parent.star")
output = steps.parallel(functions=[lib.read, lib.read])`, dir, "", `[42,42]`},
		{"ordinary import", `load("lib/parent.star", "read")
output = steps.parallel(functions=[read, read])`, dir, "", `[42,42]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := New().Execute(t.Context(), script.Spec{Source: tc.source, WorkingDirectory: tc.workDir, SourcePath: tc.sourcePath})
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, result.Value)
		})
	}
}

func TestModuleLoadSharesOrdinaryLoadCache(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`load("lib.star", "value")
a = module.load("./lib.star")
b = module.load("lib.star")
output = [value, a.value, a == b]`,
		`a = module.load("lib.star")
load("./lib.star", "value")
b = module.load("lib.star")
output = [value, a.value, a == b]`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			var reads atomic.Int32
			result, err := runSource(t, source, WithReadFile(func(string) ([]byte, error) {
				reads.Add(1)
				return []byte("value = 42"), nil
			}))
			require.NoError(t, err)
			assert.JSONEq(t, `[42,42,true]`, result.Value)
			assert.EqualValues(t, 1, reads.Load())
		})
	}
}

func TestModuleLoadConcurrentFirstLoad(t *testing.T) {
	t.Parallel()
	const count = 8
	var reads, started atomic.Int32
	ready := make(chan struct{})
	result, err := runSource(t, `
def run():
    return module.load("lib.star").value
output = steps.parallel(functions=[run for _ in range(8)], max_concurrency=8)
`, WithTaskObserver(func(event TaskEvent) {
		if event.Status == "running" && started.Add(1) == count {
			close(ready)
		}
	}), WithReadFile(func(string) ([]byte, error) {
		reads.Add(1)
		select {
		case <-ready:
			return []byte("value = 42"), nil
		case <-t.Context().Done():
			return nil, t.Context().Err()
		}
	}))
	require.NoError(t, err)
	assert.JSONEq(t, `[42,42,42,42,42,42,42,42]`, result.Value)
	assert.EqualValues(t, 1, reads.Load())
}

func TestModuleLoadCycles(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		files map[string]string
	}{
		{"direct", map[string]string{"a.star": `a = module.load("a.star")`}},
		{"indirect", map[string]string{"a.star": `b = module.load("b.star")`, "b.star": `a = module.load("a.star")`}},
		{"mixed", map[string]string{"a.star": `load("b.star", "a")`, "b.star": `a = module.load("a.star")`}},
		{"parallel during initialization", map[string]string{"a.star": `def run():
    return module.load("a.star")
values = steps.parallel(functions=[run])`}},
		{"nested parallel during initialization", map[string]string{"a.star": `def leaf():
    return module.load("b.star")
def run():
    return steps.parallel(functions=[leaf])
values = steps.parallel(functions=[run])`, "b.star": `a = module.load("a.star")`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := writeModules(t, tc.files)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			_, err := New().Execute(ctx, script.Spec{WorkingDirectory: dir, Source: `a = module.load("a.star")`})
			require.ErrorContains(t, err, "cyclic load of")
			assert.NotErrorIs(t, err, context.DeadlineExceeded)
		})
	}
}

func TestModuleLoadCrossTaskCycle(t *testing.T) {
	t.Parallel()
	var reads atomic.Int32
	ready := make(chan struct{})
	_, err := runSource(t, `
def a():
    return module.load("a.star")
def b():
    return module.load("b.star")
steps.parallel(functions=[a, b])
`, WithReadFile(func(path string) ([]byte, error) {
		if reads.Add(1) == 2 {
			close(ready)
		}
		select {
		case <-ready:
		case <-t.Context().Done():
			return nil, t.Context().Err()
		}
		if filepath.Base(path) == "a.star" {
			return []byte(`b = module.load("b.star")`), nil
		}
		return []byte(`load("a.star", "b")`), nil
	}))
	require.ErrorContains(t, err, "cyclic load of")
	assert.NotErrorIs(t, err, context.DeadlineExceeded)
}

func TestModuleLoadErrorsAndFreezing(t *testing.T) {
	t.Parallel()
	dir := writeModules(t, map[string]string{
		"lib.star": `values = []
_private = []
def change():
    _private.append(1)
`,
		"bad.star": "def broken():\n    fail(\"broken library\")\nbroken()",
	})
	for _, tc := range []struct{ source, want string }{
		{`module.load()`, "missing"},
		{`module.load(1)`, "want string"},
		{`module.load("")`, "path must not be empty"},
		{`module.load("lib.star", "extra")`, "at most"},
		{`module.load(other="lib.star")`, "unexpected keyword"},
		{`module.load("missing.star")`, "cannot read module:"},
		{`module.load("lib.star").missing`, "no .missing"},
		{`module.load("lib.star")._private`, "no ._private"},
		{`module.load("lib.star").values = []`, "assign"},
		{`module.load("lib.star").values.append(1)`, "frozen"},
		{`module.load("lib.star").change()`, "frozen"},
		{`output = module.load("lib.star")`, "cannot encode function"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			t.Parallel()
			_, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: tc.source})
			require.ErrorContains(t, err, tc.want)
		})
	}
	t.Run("traceback", func(t *testing.T) {
		t.Parallel()
		_, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: `module.load("bad.star")`})
		require.ErrorContains(t, err, "broken library")
		assert.Contains(t, errorDetails(err), filepath.Join(dir, "bad.star")+":2")
	})
}

func TestModuleLoadDryRunAndDataOutput(t *testing.T) {
	t.Parallel()
	_, err := New(WithReadFile(func(string) ([]byte, error) {
		t.Error("dry run must not read modules")
		return nil, os.ErrNotExist
	})).Execute(t.Context(), script.Spec{DryRun: true, Source: `output = struct(lib=module.load("missing.star"))`})
	require.NoError(t, err)
	result, err := runSource(t, `output = module.load("data.star")`, WithReadFile(func(string) ([]byte, error) {
		return []byte(`record = struct(value=42)`), nil
	}))
	require.NoError(t, err)
	assert.JSONEq(t, `{"record":{"value":42}}`, result.Value)
}

func TestStructConstructor(t *testing.T) {
	t.Parallel()
	result, err := runSource(t, `
def twice(x):
    return x * 2
record = struct(values=[], nested=struct(value=21), call=twice)
record.values.append(record.call(record.nested.value))
output = [struct(), record.values, dir(record), type(record), json.encode(struct(value=42))]
`)
	require.NoError(t, err)
	assert.JSONEq(t, `[{},[42],["call","nested","values"],"struct","{\"value\":42}"]`, result.Value)
	for _, tc := range []struct{ source, want string }{
		{`struct(1)`, "unexpected positional arguments"},
		{`struct(value=1).missing`, "no .missing"},
		{`struct(value=1).value = 2`, "assign"},
		{`output = struct(fn=lambda: None)`, "cannot encode function"},
		{`record = struct(values=[])
def change():
    record.values.append(1)
steps.parallel(functions=[change])`, "frozen"},
		{`def change(record):
    record.values.append(1)
steps.parallel(tasks=[steps.task(name="change", function=change, args=[struct(values=[])])])`, "frozen"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			t.Parallel()
			_, err := runSource(t, tc.source)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

// Use the session boundary to exercise retry/cancellation without relying on a
// Starlark exception handler (the language deliberately has none).
func TestModuleLoadFailureCanRetry(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"read", "evaluation", "cancellation"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			var reads atomic.Int32
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			e := New(WithReadFile(func(string) ([]byte, error) {
				if reads.Add(1) == 1 {
					switch failure {
					case "read":
						return nil, os.ErrNotExist
					case "evaluation":
						return []byte(`partial = 1; fail("failed")`), nil
					case "cancellation":
						cancel()
						return []byte(`value = 0`), nil
					}
				}
				return []byte(`value = 42`), nil
			}))
			s := newSession(t.Context(), e, &script.Spec{WorkingDirectory: t.TempDir(), DryRun: true})
			thread, stop := s.thread(ctx, "first", nil)
			defer stop()
			_, err := s.loadEntry(thread, "lib.star")
			require.Error(t, err)
			thread, stop = s.thread(t.Context(), "retry", nil)
			defer stop()
			entry, err := s.loadEntry(thread, "lib.star")
			require.NoError(t, err)
			assert.Equal(t, starlark.MakeInt(42), entry.globals["value"])
			assert.NotContains(t, entry.globals, "partial")
			assert.EqualValues(t, 2, reads.Load())
		})
	}
}

func TestModuleLoadCanceledWaiterDoesNotCancelOwner(t *testing.T) {
	t.Parallel()
	started, release := make(chan struct{}), make(chan struct{})
	var reads atomic.Int32
	e := New(WithReadFile(func(string) ([]byte, error) {
		reads.Add(1)
		close(started)
		<-release
		return []byte(`value = 42`), nil
	}))
	s := newSession(t.Context(), e, &script.Spec{WorkingDirectory: t.TempDir(), DryRun: true})
	owner, stop := s.thread(t.Context(), "owner", nil)
	defer stop()
	ownerDone := make(chan error, 1)
	go func() {
		_, err := s.loadEntry(owner, "lib.star")
		ownerDone <- err
	}()
	defer func() { close(release); require.NoError(t, <-ownerDone) }()
	<-started
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	waiter, stop := s.thread(ctx, "waiter", nil)
	defer stop()
	// A parent edge makes it possible to observe that this caller is waiting.
	parent := &moduleEntry{waits: make(map[*moduleEntry]int)}
	waiter.SetLocal(activeModuleKey, parent)
	waiterDone := make(chan error, 1)
	go func() {
		_, err := s.loadEntry(waiter, "lib.star")
		waiterDone <- err
	}()
	require.Eventually(t, func() bool {
		s.moduleMu.Lock()
		defer s.moduleMu.Unlock()
		return len(parent.waits) == 1
	}, time.Second, time.Millisecond)
	cancel()
	require.ErrorIs(t, <-waiterDone, context.Canceled)
	assert.EqualValues(t, 1, reads.Load())
}

func TestModuleConstructorsAreNotConfigurationBuiltins(t *testing.T) {
	t.Parallel()
	for _, source := range []string{`return struct(value=1)`, `return module`} {
		_, err := New().EvaluateValue(t.Context(), script.Evaluation{Source: source, Filename: "stack.yaml"})
		require.ErrorIs(t, err, errUtils.ErrStarlark)
		assert.Contains(t, err.Error(), "undefined")
	}
}

func TestModuleLoadParallelInitialization(t *testing.T) {
	t.Parallel()
	dir := writeModules(t, map[string]string{
		"a.star": `def run():
    return module.load("b.star").value
values = steps.parallel(functions=[run, run, run])`,
		"b.star": "value = 42",
	})
	result, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: `output = module.load("a.star").values`})
	require.NoError(t, err)
	assert.JSONEq(t, `[42,42,42]`, result.Value)
}

func TestModuleLoadDependencyCounts(t *testing.T) {
	t.Parallel()
	s := newSession(t.Context(), New(), &script.Spec{DryRun: true})
	a, owner, err := s.acquireModule("a", nil)
	require.NoError(t, err)
	require.True(t, owner)
	b, owner, err := s.acquireModule("b", a)
	require.NoError(t, err)
	require.True(t, owner)
	second, owner, err := s.acquireModule("b", a)
	require.NoError(t, err)
	require.False(t, owner)
	require.Same(t, b, second)
	s.releaseModuleDependency(a, b)
	assert.Equal(t, 1, a.waits[b])
	_, _, err = s.acquireModule("a", b)
	require.ErrorContains(t, err, "cyclic load of a")
	s.releaseModuleDependency(a, b)
	assert.Empty(t, a.waits)
	_, owner, err = s.acquireModule("a", b)
	require.NoError(t, err)
	assert.False(t, owner)
}

func TestModuleLoadAlreadyCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s := newSession(ctx, New(WithReadFile(func(string) ([]byte, error) {
		t.Error("a canceled load must not read the file")
		return nil, os.ErrNotExist
	})), &script.Spec{})
	thread, stop := s.thread(ctx, "canceled", nil)
	defer stop()
	_, err := s.loadEntry(thread, "missing.star")
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, s.modules)
}

func TestModuleSharedDependenciesDoNotCycle(t *testing.T) {
	t.Parallel()
	s := newSession(t.Context(), New(), &script.Spec{DryRun: true})
	a, _, err := s.acquireModule("a", nil)
	require.NoError(t, err)
	b, _, err := s.acquireModule("b", a)
	require.NoError(t, err)
	c, _, err := s.acquireModule("c", a)
	require.NoError(t, err)
	d, _, err := s.acquireModule("d", b)
	require.NoError(t, err)
	shared, _, err := s.acquireModule("d", c)
	require.NoError(t, err)
	require.Same(t, d, shared)
	outside, _, err := s.acquireModule("outside", nil)
	require.NoError(t, err)
	// Both branches reach d. Revisiting it must not mistake a diamond for a cycle.
	entry, owner, err := s.acquireModule("a", outside)
	require.NoError(t, err)
	assert.Same(t, a, entry)
	assert.False(t, owner)
	// A reverse edge really would create a cycle, including through the shared node.
	_, _, err = s.acquireModule("outside", d)
	require.ErrorContains(t, err, "cyclic load of outside")
	assert.Empty(t, d.waits)
}
