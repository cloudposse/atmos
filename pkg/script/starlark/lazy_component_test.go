package starlark

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/script"
)

// countingResolver returns a resolver that records how often each component was resolved.
func countingResolver(calls *atomic.Int32) script.ComponentResolver {
	return func(_ context.Context, ref script.ComponentRef) (*script.Component, error) {
		calls.Add(1)
		return &script.Component{
			ComponentRef: ref, Implementation: "impl-" + ref.Name, Path: "/components/" + ref.Name,
			Config: map[string]any{
				"vars": map[string]any{"name": ref.Name}, "settings": map[string]any{"team": "platform"},
				"metadata": map[string]any{"type": "real"},
			},
			Env: map[string]string{"APP_ENV": ref.Stack},
		}, nil
	}
}

func executeComponent(t *testing.T, source string, calls *atomic.Int32, withRef bool) (script.Result, error) {
	t.Helper()
	spec := script.Spec{Name: "lazy.star", Source: source, ResolveComponent: countingResolver(calls)}
	if withRef {
		spec.Component = &script.ComponentRef{Name: "api", Stack: "dev", Type: "app"}
	}
	return New().Execute(t.Context(), spec)
}

func TestCtxComponentIsNotResolvedUnlessAccessed(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`print("nothing to see")`,
		`print(ctx.component)`,
		`output = ctx.component != None`,
		`output = bool(ctx.component)`,
	} {
		var calls atomic.Int32
		_, err := executeComponent(t, source, &calls, true)
		require.NoError(t, err, source)
		assert.Zero(t, calls.Load(), source)
	}
}

func TestCtxComponentResolvesOnceAcrossAccessesAndTasks(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	result, err := executeComponent(t, `
def read(i):
    return [ctx.component.name, ctx.component.vars["name"], ctx.component.stack]
a = ctx.component.name
b = ctx.component.name
c = ctx.component.settings["team"]
output = steps.parallel(tasks = [steps.task(name = "t%d" % i, function = read, args = [i]) for i in range(6)], max_concurrency = 6)
`, &calls, true)
	require.NoError(t, err)
	assert.EqualValues(t, 1, calls.Load())
	assert.JSONEq(t, `[["api","api","dev"],["api","api","dev"],["api","api","dev"],["api","api","dev"],["api","api","dev"],["api","api","dev"]]`, result.Value)
}

func TestCtxComponentExposesTheResolvedStructAttributes(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	result, err := executeComponent(t, `
c = ctx.component
output = {
    "name": c.name, "stack": c.stack, "type": c.type, "implementation": c.implementation,
    "path": c.path, "config_has_vars": "vars" in c.config, "vars": c.vars, "settings": c.settings,
    "metadata": c.metadata, "env": c.env, "has_exec": hasattr(c, "exec"), "missing": hasattr(c, "nope"),
    "names": sorted(dir(c)),
}
`, &calls, true)
	require.NoError(t, err)
	assert.EqualValues(t, 1, calls.Load())
	assert.JSONEq(t, `{
		"name":"api","stack":"dev","type":"app","implementation":"impl-api","path":"/components/api",
		"config_has_vars":true,"vars":{"name":"api"},"settings":{"team":"platform"},"metadata":{"type":"real"},
		"env":{"APP_ENV":"dev"},"has_exec":true,"missing":false,
		"names":["config","env","exec","implementation","metadata","name","path","settings","stack","type","vars"]
	}`, result.Value)
}

func TestCtxComponentIsNoneWithoutComponentInScope(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	result, err := executeComponent(t, `output = ctx.component == None`, &calls, false)
	require.NoError(t, err)
	assert.JSONEq(t, `true`, result.Value)
	assert.Zero(t, calls.Load())
}

func TestComponentsGetIsMemoizedPerReference(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	_, err := executeComponent(t, `
a = components.get("api", "dev", "app")
b = components.get(name = "api", stack = "dev", type = "app")
def read():
    return components.get("api", "dev", "app").name
steps.parallel(functions = [read, read, read])
`, &calls, false)
	require.NoError(t, err)
	assert.EqualValues(t, 1, calls.Load())

	// Negative path: a different reference is a different lookup.
	calls.Store(0)
	_, err = executeComponent(t, `
components.get("api", "dev", "app")
components.get("api", "prod", "app")
components.get("web", "dev", "app")
`, &calls, false)
	require.NoError(t, err)
	assert.EqualValues(t, 3, calls.Load())
}

func TestCtxComponentSharesTheMemoWithComponentsGet(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	_, err := executeComponent(t, `
a = ctx.component.name
b = components.get("api", "dev", "app").name
`, &calls, true)
	require.NoError(t, err)
	assert.EqualValues(t, 1, calls.Load())
}

func TestFailedResolutionIsNotCached(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	failing := errors.New("resolver exploded")
	_, err := New().Execute(t.Context(), script.Spec{
		Component: &script.ComponentRef{Name: "api", Stack: "dev", Type: "app"},
		Source:    `ctx.component.name`,
		ResolveComponent: func(context.Context, script.ComponentRef) (*script.Component, error) {
			calls.Add(1)
			return nil, failing
		},
	})
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	require.ErrorIs(t, err, failing)
	assert.EqualValues(t, 1, calls.Load())
}

func TestComponentWaitHonorsCancellation(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	spec := script.Spec{ResolveComponent: func(_ context.Context, ref script.ComponentRef) (*script.Component, error) {
		calls.Add(1)
		close(started)
		<-release
		return &script.Component{ComponentRef: ref, Config: map[string]any{}}, nil
	}}
	s := newSession(t.Context(), New(), &spec)
	ref := script.ComponentRef{Name: "api", Stack: "dev", Type: "app"}
	owner := make(chan error, 1)
	go func() {
		_, err := s.component(t.Context(), ref)
		owner <- err
	}()
	<-started
	// Always release and join the resolver, including when the assertion fails.
	defer func() {
		close(release)
		require.NoError(t, <-owner)
	}()

	ctx, cancel := context.WithCancel(t.Context())
	waiter := make(chan error, 1)
	go func() {
		_, err := s.component(ctx, ref)
		waiter <- err
	}()
	cancel()
	select {
	case err := <-waiter:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("canceled component lookup waited for an unrelated resolver")
	}
	assert.EqualValues(t, 1, calls.Load())
}

func TestComponentsGetHonorsTaskDeadline(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var calls atomic.Int32
	_, err := New().Execute(ctx, script.Spec{
		Source: `
def read():
    return components.get(name="api", stack="dev", type="app").name
steps.parallel(tasks=[steps.task(name="read", function=read, timeout="1s")])`,
		ResolveComponent: func(ctx context.Context, _ script.ComponentRef) (*script.Component, error) {
			calls.Add(1)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, errUtils.ErrStarlarkTaskTimeout)
	assert.NoError(t, ctx.Err(), "the task deadline must reach the resolver before the session deadline")
	assert.EqualValues(t, 1, calls.Load())
}
