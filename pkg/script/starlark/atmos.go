package starlark

import (
	"path/filepath"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script"
	atmosmodule "github.com/cloudposse/atmos/pkg/script/starlark/stdlib/atmos"
)

// WithAtmosExecutable supplies the current Atmos binary lookup for embedded hosts.
func WithAtmosExecutable(executable func() (string, error)) Option {
	defer perf.Track(nil, "starlark.WithAtmosExecutable")()
	return func(e *Engine) { e.executable = executable }
}

// WithAtmosCommands supplies an immutable snapshot of the host CLI's command registry.
func WithAtmosCommands(catalog *flags.CommandCatalog) Option {
	defer perf.Track(nil, "starlark.WithAtmosCommands")()
	return func(e *Engine) { e.commands = catalog }
}

func (s *session) atmosModule() starlark.Value {
	return atmosmodule.New("", s.engine.commands, s.runAtmos)
}

func (s *session) runAtmos(thread *starlark.Thread, argv []string, opts atmosmodule.Options, allowPlanChanges bool) (starlark.Value, error) {
	binary, err := s.engine.executable()
	if err != nil {
		return nil, failWith(errUtils.ErrStarlark, err, "locate Atmos executable: %s", err)
	}
	env, err := processEnv(s.osEnvironment(thread, s.executionScope(thread).env), opts.Env)
	if err != nil {
		return nil, err
	}
	dir := opts.Dir
	base := s.spec.AtmosWorkingDirectory
	if _, ok := threadContext(thread).Value(sandboxScopeKey{}).(*sandboxScope); ok {
		base = s.executionScope(thread).dir
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(base, dir)
	}
	runOpts := &runOptions{check: opts.Check, output: opts.Output}
	stream, err := runOpts.streaming()
	if err != nil {
		return nil, err
	}
	return s.runProcess(thread, &processCall{
		stdin: opts.Stdin, argv: append([]string{binary}, argv...), dir: dir, env: env,
		check: opts.Check, stream: stream, viewport: runOpts.viewport(), allowPlanChanges: allowPlanChanges, dataHint: atmosDataHint,
		atmos: true, label: script.AtmosLabel(argv),
	})
}
