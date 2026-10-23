package starlark

import (
	"context"
	"errors"
	"os"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/git/sandbox"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script"
)

type sandboxScopeKey struct{}

// executionScope is carried by context, not session mutation or os.Chdir. Parallel
// tasks inherit it while independent script invocations keep their own directories.
func (s *session) executionScope(t *starlark.Thread) processScope {
	if scope, ok := threadContext(t).Value(sandboxScopeKey{}).(processScope); ok {
		return scope
	}
	return processScope{dir: s.spec.WorkingDirectory, env: s.spec.ProcessEnv}
}

// WithGitRunner supplies Git plumbing for isolated checkouts, including test doubles.
func WithGitRunner(runner atmosgit.Runner) Option {
	defer perf.Track(nil, "starlark.WithGitRunner")()
	return func(e *Engine) { e.gitRunner = runner }
}

func (s *session) gitModule() starlark.Value {
	return module("git", starlark.StringDict{"sandbox": starlark.NewBuiltin("git.sandbox", s.gitSandbox)})
}

// gitSandbox owns cleanup and reconciliation even when a callback fails. Callback
// defers run inside the sandbox before reconciliation; cancellation never applies fixes.
func (s *session) gitSandbox(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (_ starlark.Value, resultErr error) {
	opts, err := s.sandboxArguments(t, b, args, kwargs)
	if err != nil {
		return nil, err
	}
	parent := s.executionScope(t)
	sourceEnv, err := processEnv(parent.env, nil)
	if err != nil {
		return nil, err
	}
	ctx := threadContext(t)
	box, err := s.openSandbox(ctx, t, sandbox.Options{Directory: parent.dir, Scope: opts.scope, BaseRef: opts.base, Env: sourceEnv, Runner: s.engine.gitRunner, Stderr: s.writer(t, stderrStream)})
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, box.Close()) }()
	env, err := sandboxEnvironment(sourceEnv, box.Path, &opts)
	if err != nil {
		return nil, err
	}
	return s.runSandbox(ctx, t, box, &opts, env)
}

type sandboxArguments struct {
	run                starlark.Callable
	scope, base, fixes string
	failOnChanges      bool
	env                *starlark.Dict
	paths              []string
}

func (s *session) sandboxArguments(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (sandboxArguments, error) {
	var run starlark.Callable
	scope, base, fixes := "staged", "origin/main", "none"
	failOnChanges := true
	var extraEnv *starlark.Dict
	var pathPrepend starlark.Value = starlark.Tuple{}
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "run", &run, "scope?", &scope, "base_ref?", &base, "fixes?", &fixes, "fail_on_changes?", &failOnChanges, "env?", &extraEnv, "path_prepend?", &pathPrepend); err != nil {
		return sandboxArguments{}, err
	}
	if fixes != "none" && fixes != "unstaged" {
		return sandboxArguments{}, invalidArg("git.sandbox: fixes must be none or unstaged")
	}
	if _, nested := threadContext(t).Value(sandboxScopeKey{}).(processScope); nested {
		return sandboxArguments{}, invalidArg("git.sandbox cannot be nested")
	}
	paths, err := stringSequence(pathPrepend, "path_prepend")
	if err != nil {
		return sandboxArguments{}, err
	}
	for i, path := range paths {
		paths[i] = s.filesystemPath(t, path)
	}
	return sandboxArguments{run, scope, base, fixes, failOnChanges, extraEnv, paths}, nil
}

func sandboxEnvironment(sourceEnv []string, path string, opts *sandboxArguments) ([]string, error) {
	env, err := processEnv(sandbox.CleanEnvironment(sourceEnv), opts.env)
	if err != nil {
		return nil, err
	}
	if len(opts.paths) > 0 {
		for _, entry := range env {
			if key, value, ok := strings.Cut(entry, "="); ok && strings.EqualFold(key, "PATH") {
				opts.paths = append(opts.paths, value)
				break
			}
		}
		env = script.ProcessEnvironment(env, map[string]string{"PATH": strings.Join(opts.paths, string(os.PathListSeparator))})
	}
	// Config discovery and nested Atmos calls must stay inside the temporary checkout.
	env = script.ProcessEnvironment(env, map[string]string{"ATMOS_BASE_PATH": path, "ATMOS_CLI_CONFIG_PATH": path, "ATMOS_VERSION_CHECK_ENABLED": "false"})
	return env, nil
}

func (s *session) runSandbox(ctx context.Context, t *starlark.Thread, box *sandbox.Sandbox, opts *sandboxArguments, env []string) (starlark.Value, error) {
	ctx = context.WithValue(ctx, sandboxScopeKey{}, processScope{dir: box.Path, env: env})
	callback, stop := s.thread(ctx, "git.sandbox", threadOutput(t))
	defer stop()
	callback.SetLocal(stepLibraryKey, forkSteps(threadSteps(t)))
	value, runErr := starlark.Call(callback, opts.run, starlark.Tuple{sandboxValue(box)}, nil)
	runErr = s.runDeferred(ctx, callback, runErr)
	changed, finishErr := box.Finish(opts.fixes == "unstaged")
	if changed && opts.failOnChanges {
		finishErr = errors.Join(finishErr, fail(errUtils.ErrStarlark, "checks require changes; review fixes and restage, or run the formatting command"))
	}
	if err := errors.Join(runErr, finishErr); err != nil {
		return nil, err
	}
	return value, nil
}

func sandboxValue(box *sandbox.Sandbox) starlark.Value {
	stringsList := func(paths []string) *starlark.List {
		values := make([]starlark.Value, len(paths))
		for i, p := range paths {
			values[i] = starlark.String(p)
		}
		return starlark.NewList(values)
	}
	method := func(name string, fn func([]string) error) *starlark.Builtin {
		return starlark.NewBuiltin(name, func(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var paths starlark.Value
			if err := starlark.UnpackArgs(b.Name(), args, kwargs, "paths", &paths); err != nil {
				return nil, err
			}
			if err := threadContext(t).Err(); err != nil {
				return nil, err
			}
			values, err := stringSequence(paths, "paths")
			if err != nil {
				return nil, err
			}
			return starlark.None, fn(values)
		})
	}
	value := starlarkstruct.FromStringDict(starlark.String("git_sandbox"), starlark.StringDict{
		"root": starlark.String(box.Root), "path": starlark.String(box.Path), "scope": starlark.String(box.Scope), "base": starlark.String(box.Base),
		"files": stringsList(box.Files), "added": stringsList(box.Added),
		"track": method("sandbox.track", box.Track), "copy": method("sandbox.copy", box.Copy),
	})
	value.Freeze()
	return value
}

func stringSequence(value starlark.Value, name string) ([]string, error) {
	if _, ok := value.(starlark.String); ok {
		return nil, invalidArg("%s must be a list of strings", name)
	}
	values, err := sequence(value)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(values))
	for i, v := range values {
		str, ok := starlark.AsString(v)
		if !ok {
			return nil, invalidArg("%s must contain strings", name)
		}
		result[i] = str
	}
	return result, nil
}
