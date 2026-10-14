// Package starlark embeds Starlark with cancellable, isolated parallel tasks.
package starlark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	starjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/script"
	climodule "github.com/cloudposse/atmos/pkg/script/starlark/stdlib/cli"
	regexmodule "github.com/cloudposse/atmos/pkg/script/starlark/stdlib/regex"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Engine contains immutable host services shared by independent invocations.
type Engine struct {
	runner     process.Runner
	clock      retry.Clock
	readFile   func(string) ([]byte, error)
	observe    func(TaskEvent)
	executable func() (string, error)
	logger     Logger
	commands   *flags.CommandCatalog
}

// Option configures the host services. Injected services must be concurrency safe.
type Option func(*Engine)

// WithProcessRunner supplies the process execution boundary, including test mocks.
func WithProcessRunner(runner process.Runner) Option {
	defer perf.Track(nil, "starlark.WithProcessRunner")()

	return func(e *Engine) { e.runner = runner }
}

// WithRetryClock supplies time for retry backoff, including deterministic tests.
func WithRetryClock(clock retry.Clock) Option {
	defer perf.Track(nil, "starlark.WithRetryClock")()

	return func(e *Engine) { e.clock = clock }
}

// WithReadFile supplies local module and fs.read_file contents.
func WithReadFile(readFile func(string) ([]byte, error)) Option {
	defer perf.Track(nil, "starlark.WithReadFile")()

	return func(e *Engine) { e.readFile = readFile }
}

// WithTaskObserver receives serialized branch lifecycle events.
func WithTaskObserver(observe func(TaskEvent)) Option {
	defer perf.Track(nil, "starlark.WithTaskObserver")()

	return func(e *Engine) { e.observe = observe }
}

// New creates an embedded engine using Atmos process execution and retry services.
func New(opts ...Option) *Engine {
	defer perf.Track(nil, "starlark.New")()

	e := &Engine{runner: process.NewDefaultRunner(), readFile: os.ReadFile, executable: os.Executable}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func init() { script.Register("starlark", New()) }

type session struct {
	engine  *Engine
	spec    script.Spec
	ctx     context.Context
	globals starlark.StringDict
	modules map[string]starlark.StringDict
	loading map[string]bool
	mu      sync.Mutex

	componentMu sync.Mutex
	components  map[script.ComponentRef]*componentEntry
	tools       map[string]string
	toolDirs    []string
}

// Execute runs a script and returns its optional top-level output value: a string is
// returned as-is, any other value is JSON-encoded.
//
//nolint:gocritic // Copy the invocation so normalizing paths and streams cannot mutate its caller.
func (e *Engine) Execute(ctx context.Context, spec script.Spec) (script.Result, error) {
	defer perf.Track(nil, "starlark.Engine.Execute")()

	if ctx == nil {
		ctx = context.Background()
	}
	if err := prepareSpec(&spec); err != nil {
		return script.Result{}, err
	}
	s := newSession(ctx, e, &spec)
	if spec.DryRun {
		return script.Result{}, s.check(ctx)
	}
	if err := ctx.Err(); err != nil {
		return script.Result{}, err
	}
	if err := validateWorkingDirectory(spec.WorkingDirectory); err != nil {
		return script.Result{}, scriptError(ctx, err, s.spec.ProjectRoot)
	}
	thread, stop := s.thread(ctx, spec.Name, nil)
	defer stop()
	if err := s.componentContext(thread); err != nil {
		return script.Result{}, scriptError(ctx, err, s.spec.ProjectRoot)
	}
	globals, err := starlark.ExecFileOptions(fileOptions, thread, programName(&spec), spec.Source, s.globals)
	if err != nil {
		return script.Result{}, scriptError(ctx, err, s.spec.ProjectRoot)
	}
	return s.output(ctx, thread, globals)
}

func newSession(ctx context.Context, e *Engine, spec *script.Spec) *session {
	s := &session{
		engine: e, spec: *spec, ctx: ctx, modules: make(map[string]starlark.StringDict),
		loading: make(map[string]bool), components: make(map[script.ComponentRef]*componentEntry),
	}
	s.globals = s.predeclared()
	return s
}

// check parses and resolves the source without running anything, for dry runs.
func (s *session) check(ctx context.Context) error {
	s.globals["ctx"] = starlark.None
	if _, _, err := starlark.SourceProgramOptions(fileOptions, programName(&s.spec), s.spec.Source, s.globals.Has); err != nil {
		return scriptError(ctx, err, s.spec.ProjectRoot)
	}
	return nil
}

// output converts the top-level output global: strings pass through raw, everything else is JSON.
func (s *session) output(ctx context.Context, thread *starlark.Thread, globals starlark.StringDict) (script.Result, error) {
	value, ok := globals["output"]
	if !ok {
		return script.Result{}, nil
	}
	if text, isString := value.(starlark.String); isString {
		return script.Result{Value: string(text), HasOutput: true}, nil
	}
	encoded, err := starlark.Call(thread, starjson.Module.Members["encode"], starlark.Tuple{value}, nil)
	if err != nil {
		reason := strings.TrimPrefix(evalMessage(err), "json.encode: ")
		failed := fail(errUtils.ErrStarlarkOutputEncode, "`output` must be a string or JSON-encodable value: %s", reason)
		failed = errUtils.Build(failed).
			WithHintf("Assign a string, number, bool, list, dict, or None to `output` in step %q.", s.spec.Name).Err()
		return script.Result{}, scriptError(ctx, failed, s.spec.ProjectRoot)
	}
	return script.Result{Value: string(encoded.(starlark.String)), HasOutput: true}, nil
}

func prepareSpec(spec *script.Spec) error {
	if spec.Stdout == nil {
		spec.Stdout = io.Discard
	}
	if spec.Stderr == nil {
		spec.Stderr = io.Discard
	}
	if spec.Name == "" {
		spec.Name = "<script>"
	}
	dir, err := filepath.Abs(spec.WorkingDirectory)
	if err != nil {
		return err
	}
	spec.WorkingDirectory = dir
	if spec.ProjectRoot != "" {
		if spec.ProjectRoot, err = filepath.Abs(spec.ProjectRoot); err != nil {
			return err
		}
	}
	if spec.SourcePath != "" {
		if spec.SourcePath, err = filepath.Abs(spec.SourcePath); err != nil {
			return err
		}
		if spec.File == nil {
			spec.File = &script.File{Path: spec.SourcePath}
		}
	}
	spec.AtmosWorkingDirectory, err = filepath.Abs(spec.AtmosWorkingDirectory)
	return err
}

// programName is the filename Starlark records for the entry script. A script read from a file is
// named by its absolute path, so tracebacks name the real file and load() resolves relative to it;
// an inline script is named by its step.
func programName(spec *script.Spec) string {
	if spec.SourcePath != "" {
		return spec.SourcePath
	}
	return spec.Name
}

// validateWorkingDirectory fails fast, before any code runs, when the directory is unusable.
func validateWorkingDirectory(dir string) error {
	info, err := os.Stat(dir)
	var failed error
	switch {
	case errors.Is(err, fs.ErrNotExist):
		failed = failWith(errUtils.ErrStarlark, err, "working directory %q does not exist", dir)
	case err != nil:
		failed = failWith(errUtils.ErrStarlark, err, "working directory %q is not accessible: %s", dir, err)
	case !info.IsDir():
		failed = fail(errUtils.ErrStarlark, "working directory %q is not a directory", dir)
	default:
		return nil
	}
	return errUtils.Build(failed).WithHint("Check the `working_directory` setting; it must name an existing directory.").Err()
}

const contextKey = "atmos.starlark.context"

// Creates an interpreter thread bound to ctx. A nil task output marks the main thread, whose
// output goes straight to the session streams.
func (s *session) thread(ctx context.Context, name string, out *taskOutput) (*starlark.Thread, func()) {
	t := &starlark.Thread{Name: name, Load: s.load, Print: func(t *starlark.Thread, msg string) {
		_, _ = fmt.Fprintln(s.writer(t, stdoutStream), msg)
	}}
	limitRecursion(t)
	t.SetLocal(contextKey, ctx)
	if out != nil {
		t.SetLocal(outputKey, out)
	}
	stop := context.AfterFunc(ctx, func() { t.Cancel(ctx.Err().Error()) })
	return t, func() { stop() }
}

func threadContext(t *starlark.Thread) context.Context { return t.Local(contextKey).(context.Context) }

func module(name string, members starlark.StringDict) starlark.Value {
	return &starlarkstruct.Module{Name: name, Members: members}
}

func stringDict(values map[string]string) *starlark.Dict {
	d := starlark.NewDict(len(values))
	for key, value := range values {
		_ = d.SetKey(starlark.String(key), starlark.String(value))
	}
	d.Freeze()
	return d
}

func (s *session) predeclared() starlark.StringDict {
	return starlark.StringDict{
		"cli": climodule.New(func(t *starlark.Thread, command script.CommandSpec) (script.CommandInput, error) {
			if s.spec.ParseCommand == nil || t.Local(outputKey) != nil {
				return script.CommandInput{}, fail(errUtils.ErrStarlarkInvalidArgument, "cli.command is available only in a standalone script's main thread")
			}
			return s.spec.ParseCommand(threadContext(t), command)
		}),
		"dependencies": module("dependencies", starlark.StringDict{"tools": starlark.NewBuiltin("dependencies.tools", s.installTools)}),
		"atmos":        s.atmosModule(),
		"components":   module("components", starlark.StringDict{"get": starlark.NewBuiltin("components.get", s.getComponent)}),
		"ui":           module("ui", s.uiMembers()),
		"env":          stringDict(s.spec.Env),
		"json":         starjson.Module,
		"fs":           module("fs", starlark.StringDict{"read_file": starlark.NewBuiltin("fs.read_file", s.readFile)}),
		"regex":        regexmodule.New(),
		"steps": module("steps", starlark.StringDict{
			"task":     starlark.NewBuiltin("steps.task", newTask),
			"parallel": starlark.NewBuiltin("steps.parallel", s.parallel),
		}),
		"exec": module("exec", starlark.StringDict{"run": starlark.NewBuiltin("exec.run", s.exec)}),
		"log":  module("log", s.logMembers()),
	}
}

// uiMembers builds ui.info/success/warning. Each call resolves its destination from the calling
// thread so messages from parallel tasks are line-atomic and prefixed like other task output.
func (s *session) uiMembers() starlark.StringDict {
	writers := map[string]func(*ui.Output, string){
		"info":    (*ui.Output).Info,
		"success": (*ui.Output).Success,
		"warning": (*ui.Output).Warning,
	}
	members := starlark.StringDict{}
	for name, write := range writers {
		members[name] = starlark.NewBuiltin("ui."+name, func(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var message string
			if err := starlark.UnpackArgs(b.Name(), args, kwargs, "message", &message); err != nil {
				return nil, err
			}
			write(ui.New(s.writer(t, stderrStream)), message)
			return starlark.None, nil
		})
	}
	return members
}

// Evaluates local modules once. Absolute paths are used as-is; relative paths, including
// parent-directory segments, resolve against the importing file (the entry script's own file when
// it has one), or working_directory for inline entrypoints.
// Starlark loads are top-level, so module evaluation finishes before any imported function
// can be dispatched. Modules are cached by cleaned absolute path.
func (s *session) load(thread *starlark.Thread, name string) (starlark.StringDict, error) {
	path := filepath.Clean(name)
	if !filepath.IsAbs(name) {
		dir := s.spec.WorkingDirectory
		if thread.CallStackDepth() > 0 {
			caller := thread.CallFrame(0).Pos.Filename()
			if filepath.IsAbs(caller) {
				dir = filepath.Dir(caller)
			}
		}
		path = filepath.Clean(filepath.Join(dir, name))
	}
	if s.loading[path] {
		return nil, fail(errUtils.ErrStarlark, "cyclic load of %s", path)
	}
	if globals, ok := s.modules[path]; ok {
		return globals, nil
	}
	s.loading[path] = true
	defer delete(s.loading, path)
	source, err := s.engine.readFile(path)
	if err != nil {
		return nil, failWith(errUtils.ErrStarlark, err, "cannot read module: %s", err)
	}
	globals, err := starlark.ExecFileOptions(fileOptions, thread, path, source, s.globals)
	if err != nil {
		return nil, err
	}
	globals.Freeze()
	s.modules[path] = globals
	return globals, nil
}
