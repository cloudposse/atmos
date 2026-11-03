// Package starlark embeds Starlark with cancellable, isolated parallel tasks.
package starlark

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"

	starjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/flags"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/script"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/threadcontext"
	climodule "github.com/cloudposse/atmos/pkg/script/starlark/stdlib/cli"
	configmodule "github.com/cloudposse/atmos/pkg/script/starlark/stdlib/config"
	digestmodule "github.com/cloudposse/atmos/pkg/script/starlark/stdlib/digest"
	regexmodule "github.com/cloudposse/atmos/pkg/script/starlark/stdlib/regex"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Engine contains immutable host services shared by independent invocations.
type Engine struct {
	gitRunner  atmosgit.Runner
	schemaHTTP *http.Client
	filesystem api.FileSystem
	runner     process.Runner
	clock      retry.Clock
	readFile   func(string) ([]byte, error)
	writeFile  func(string, []byte, os.FileMode) error
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

// WithReadFile supplies local module, configuration, schema, and fs.read_file contents.
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

	e := &Engine{schemaHTTP: &http.Client{}, filesystem: api.LocalFileSystem{}, runner: process.NewDefaultRunner(), readFile: os.ReadFile, writeFile: atomicWriteFile, executable: os.Executable}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func init() {
	script.Register("starlark", New(), script.WithExtensions(".star"), script.WithAtmosShebang())
}

type session struct {
	engine    *Engine
	spec      script.Spec
	ctx       context.Context
	globals   starlark.StringDict
	mu        sync.Mutex
	interrupt *interruptWatch

	moduleMu sync.Mutex
	modules  map[string]*moduleEntry

	componentMu sync.Mutex
	components  map[script.ComponentRef]*componentEntry
	tools       *script.Tools
	osEnv       map[string]string

	// ciReporter backs the ci module. It is never nil: hosts without a CI reporter get the
	// local-only one. ciCtxValue caches the lazily resolved ci.context.
	ciReporter ci.Reporter
	ciCtxOnce  sync.Once
	ciCtxValue starlark.Value
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
	if err := script.PrepareSpec(&spec); err != nil {
		return script.Result{}, err
	}
	s := newSession(ctx, e, &spec)
	return s.execute(ctx)
}

// execute validates and runs a prepared invocation, then finishes deferred calls and output.
func (s *session) execute(ctx context.Context) (script.Result, error) {
	spec := &s.spec
	if spec.DryRun {
		return script.Result{}, s.check(ctx)
	}
	if err := ctx.Err(); err != nil {
		return script.Result{}, err
	}
	if err := script.ValidateWorkingDirectory(spec.WorkingDirectory); err != nil {
		return script.Result{}, scriptError(ctx, err, s.spec.ProjectRoot)
	}
	// Ctrl-C cancels this session instead of ending the process, so deferred calls get to run.
	ctx, interrupt := watchInterrupt(ctx)
	defer interrupt.stop()
	s.interrupt = interrupt
	s.ctx = ctx
	thread, stop := s.thread(ctx, spec.Name, nil)
	defer stop()
	thread.SetLocal(stepLibraryKey, forkSteps(spec.Steps))
	if err := s.componentContext(thread); err != nil {
		return script.Result{}, scriptError(ctx, err, s.spec.ProjectRoot)
	}
	globals, err := execAutomation(thread, programName(spec), spec.Source, s.globals)
	if err != nil && interrupt.interrupted() {
		return script.Result{}, s.finishInterrupted(ctx, thread)
	}
	// Help ends the script early on purpose. It is a success: deferred calls still run, and no
	// output value is printed.
	helpShown := errors.Is(err, errUtils.ErrScriptHelpShown)
	if helpShown {
		err = nil
	}
	if err = s.runDeferred(ctx, thread, err); err != nil {
		return script.Result{}, scriptError(ctx, err, s.spec.ProjectRoot)
	}
	if helpShown {
		return script.Result{}, nil
	}
	return s.output(ctx, thread, globals)
}

func newSession(ctx context.Context, e *Engine, spec *script.Spec) *session {
	s := &session{
		engine: e, spec: *spec, ctx: ctx, modules: make(map[string]*moduleEntry),
		components: make(map[script.ComponentRef]*componentEntry),
		tools:      script.NewTools(spec.InstallTools),
	}
	s.ciReporter = spec.CI
	if s.ciReporter == nil {
		// Every production host wires a configured reporter. A nil one means a host forgot, and
		// the local-only reporter reads every CI gate as off, so make the regression visible.
		// Dry runs only parse the script and never reach the reporter, so they stay quiet.
		if !spec.DryRun {
			log.Warn("script host supplied no CI reporter; CI gates read as off", "script", spec.Name)
		}
		s.ciReporter = ci.NewReporter(nil)
	}
	s.globals = s.predeclared()
	return s
}

// check parses and resolves the source without running anything, for dry runs.
func (s *session) check(ctx context.Context) error {
	s.globals["ctx"] = starlark.None
	if _, err := compileAutomation(programName(&s.spec), s.spec.Source, s.globals); err != nil {
		return scriptError(ctx, err, s.spec.ProjectRoot)
	}
	return nil
}

// output converts the top-level output global: strings pass through raw, everything else is JSON.
// None means "no output", so a script whose command callback returns nothing prints nothing.
func (s *session) output(ctx context.Context, thread *starlark.Thread, globals starlark.StringDict) (script.Result, error) {
	value, ok := globals["output"]
	if !ok || value == starlark.None {
		return script.Result{}, nil
	}
	if text, isString := value.(starlark.String); isString {
		return script.Result{Value: string(text), HasOutput: true}, nil
	}
	encoded, err := starlark.Call(thread, starjson.Module.Members["encode"], starlark.Tuple{value}, nil)
	if err != nil {
		reason := strings.TrimPrefix(evalMessage(err), "json.encode: ")
		failed := fail(errUtils.ErrStarlarkOutputEncode, "`output` must be a string or JSON-encodable value: %s", reason)
		failed = errUtils.Build(failed).WithHint(s.outputHint()).Err()
		return script.Result{}, scriptError(ctx, failed, s.spec.ProjectRoot)
	}
	return script.Result{Value: string(encoded.(starlark.String)), HasOutput: true}, nil
}

// outputHint names what `output` may hold. None is deliberately absent: it means no output.
func (s *session) outputHint() string {
	const accepted = "Assign a string, number, bool, list, or dict to `output`"
	if s.spec.ParseCommand != nil {
		return fmt.Sprintf("%s in script %q, or leave it unset (or None) to print nothing.", accepted, s.spec.Name)
	}
	return fmt.Sprintf("%s in step %q, or leave it unset (or None) to produce no output.", accepted, s.spec.Name)
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

// Creates an interpreter thread bound to ctx. A nil task output marks the main thread, whose
// output goes straight to the session streams.
func (s *session) thread(ctx context.Context, name string, out *taskOutput) (*starlark.Thread, func()) {
	t := &starlark.Thread{Name: name, Load: s.load, Print: func(t *starlark.Thread, msg string) {
		_, _ = fmt.Fprintln(s.writer(t, stdoutStream), msg)
	}}
	limitRecursion(t)
	threadcontext.Set(ctx, t)
	t.SetLocal(activeModuleKey, ctx.Value(moduleContextKey{}))
	if out != nil {
		t.SetLocal(outputKey, out)
	}
	stop := context.AfterFunc(ctx, func() { t.Cancel(ctx.Err().Error()) })
	return t, func() { stop() }
}

func threadContext(t *starlark.Thread) context.Context { return threadcontext.Get(t) }

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
		"config": configmodule.New(configmodule.Host{ReadFile: s.engine.readFile, ResolvePath: s.modulePath, HTTPClient: s.engine.schemaHTTP}),
		"schema": configmodule.Schema(),
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
		"fail":   starlark.NewBuiltin("fail", raiseFailure),
		"module": module("module", starlark.StringDict{"load": starlark.NewBuiltin("module.load", s.loadModule)}),
		"sum":    starlark.NewBuiltin("sum", numericSum),
		"round":  starlark.NewBuiltin("round", numericRound),
		"defer":  starlark.NewBuiltin("defer", deferCall),
		"errors": module("errors", starlark.StringDict{"build": starlark.NewBuiltin("errors.build", buildError)}),
		"cli": climodule.New(func(t *starlark.Thread, command script.CommandSpec) (script.CommandInput, error) {
			if s.spec.ParseCommand == nil || t.Local(outputKey) != nil {
				return script.CommandInput{}, fail(errUtils.ErrStarlarkInvalidArgument, "cli.command is available only in a standalone script's main thread")
			}
			input, err := s.spec.ParseCommand(threadContext(t), command)
			if errors.Is(err, errUtils.ErrScriptUsage) {
				// The host already presented this as a usage error; keep it from being
				// re-reported as a script crash with a traceback.
				return input, &script.UsageFailure{Err: err}
			}
			return input, err
		}),
		"dependencies": module("dependencies", starlark.StringDict{"tools": starlark.NewBuiltin("dependencies.tools", s.installTools)}),
		"atmos":        s.atmosModule(),
		"components":   module("components", starlark.StringDict{"get": starlark.NewBuiltin("components.get", s.getComponent)}),
		"ui":           module("ui", s.uiMembers()),
		"env":          stringDict(s.spec.Env),
		"json":         starjson.Module,
		"fs":           s.filesystemModule(),
		"git":          s.gitModule(),
		"yaml":         yamlModule(),
		"regex":        regexmodule.New(),
		"steps":        s.stepsModule(),
		"cast":         s.castModule(),
		"digest":       digestmodule.New(),
		"os": module("os", starlark.StringDict{
			"arch":     starlark.String(runtime.GOARCH),
			"chdir":    starlark.NewBuiltin("os.chdir", s.chdir),
			"environ":  starlark.NewBuiltin("os.environ", s.environ),
			"getcwd":   starlark.NewBuiltin("os.getcwd", s.getcwd),
			"getenv":   starlark.NewBuiltin("os.getenv", s.getenv),
			"platform": starlark.String(runtime.GOOS),
			"setenv":   starlark.NewBuiltin("os.setenv", s.setenv),
		}),
		"exec": module("exec", starlark.StringDict{
			"run":   starlark.NewBuiltin("exec.run", s.exec),
			"which": starlark.NewBuiltin("exec.which", s.which),
		}),
		"log": module("log", s.logMembers()),
		"ci":  s.ciModule(),
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
