package atmos

import (
	"sort"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/convert"
)

const (
	atmosFlagPrefix     = "-"
	streamOutput        = "stream"
	workingDirectoryArg = "working_directory?"
	environmentArg      = "env?"
	outputArg           = "output?"
	checkArg            = "check?"
)

// Options are invocation policies supplied to the host process runner.
type Options struct {
	Dir    string
	Env    *starlark.Dict
	Output string
	Check  bool
}

// Run delegates execution to the embedding host, preserving its cancellation,
// output routing, masking, and process environment policies.
type Run func(*starlark.Thread, []string, Options, bool) (starlark.Value, error)

type binding struct {
	directory string
	catalog   *flags.CommandCatalog
	run       Run
}

// New returns the Atmos module backed by the host's registered command catalog.
func New(directory string, catalog *flags.CommandCatalog, run Run) starlark.Value {
	defer perf.Track(nil, "atmos.New")()

	s := &binding{directory: directory, catalog: catalog, run: run}
	return s.module()
}

func (s *binding) module() starlark.Value {
	members := starlark.StringDict{
		"run":       starlark.NewBuiltin("atmos.run", s.atmosRun),
		"terraform": starlark.NewBuiltin("atmos.terraform", s.atmosComponentCommand),
		"helm":      starlark.NewBuiltin("atmos.helm", s.atmosComponentCommand),
		"toolchain": starlark.NewBuiltin("atmos.toolchain", s.atmosToolchain),
	}
	for _, name := range s.catalog.Names() {
		if _, reserved := members[name]; reserved {
			continue
		}
		members[name] = starlark.NewBuiltin("atmos."+name, s.atmosCommand)
	}
	return &starlarkstruct.Module{Name: "atmos", Members: members}
}

func (s *binding) atmosToolchain(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command, tool string
	var flags *starlark.Dict
	var extra starlark.Value = starlark.Tuple{}
	opts := Options{Dir: s.directory, Output: streamOutput, Check: true}
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "command", &command, "tool?", &tool,
		"flags?", &flags, "args?", &extra, workingDirectoryArg, &opts.Dir,
		environmentArg, &opts.Env, outputArg, &opts.Output, checkArg, &opts.Check); err != nil {
		return nil, err
	}
	if command == "" || strings.HasPrefix(command, atmosFlagPrefix) || strings.HasPrefix(tool, atmosFlagPrefix) {
		return nil, convert.InvalidArgument("command must be nonempty; command and tool must not be flags")
	}
	argv := []string{"toolchain", command}
	if tool != "" {
		argv = append(argv, tool)
	}
	flagArgs, err := s.atmosFlags([]string{"toolchain", command}, flags)
	if err != nil {
		return nil, err
	}
	extraArgs, err := atmosExtraArgs(extra)
	if err != nil {
		return nil, err
	}
	argv = append(argv, flagArgs...)
	argv = append(argv, extraArgs...)
	return s.run(thread, argv, opts, false)
}

func (s *binding) atmosRun(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var argv starlark.Value
	opts := Options{Dir: s.directory, Output: streamOutput, Check: true}
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "argv", &argv, workingDirectoryArg, &opts.Dir,
		environmentArg, &opts.Env, outputArg, &opts.Output, checkArg, &opts.Check); err != nil {
		return nil, err
	}
	command, err := atmosExtraArgs(argv)
	if err != nil {
		return nil, err
	}
	if len(command) == 0 || command[0] == "" {
		return nil, convert.InvalidArgument("argv must contain a command")
	}
	return s.run(thread, command, opts, false)
}

func (s *binding) atmosComponentCommand(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command, component, stack string
	var flags *starlark.Dict
	var extra starlark.Value = starlark.Tuple{}
	opts := Options{Dir: s.directory, Output: streamOutput, Check: true}
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "command", &command, "component", &component, "stack", &stack,
		"flags?", &flags, "args?", &extra, workingDirectoryArg, &opts.Dir,
		environmentArg, &opts.Env, outputArg, &opts.Output, checkArg, &opts.Check); err != nil {
		return nil, err
	}
	for _, value := range []string{command, component, stack} {
		if value == "" || strings.HasPrefix(value, atmosFlagPrefix) {
			return nil, convert.InvalidArgument("command, component, and stack must be nonempty values, not flags")
		}
	}
	provider := strings.TrimPrefix(b.Name(), "atmos.")
	flagArgs, err := s.atmosFlags([]string{provider, command}, flags)
	if err != nil {
		return nil, err
	}
	argv := append([]string{provider, command, component, "--stack=" + stack}, flagArgs...)
	extraArgs, err := atmosExtraArgs(extra)
	if err != nil {
		return nil, err
	}
	argv = append(argv, extraArgs...)
	return s.run(thread, argv, opts, provider == "terraform" && command == "plan" && detailedPlanRequested(argv))
}

func atmosExtraArgs(extra starlark.Value) ([]string, error) {
	values, err := convert.Sequence(extra)
	if err != nil {
		return nil, err
	}
	argv := make([]string, 0, len(values))
	for _, value := range values {
		arg, ok := starlark.AsString(value)
		if !ok {
			return nil, convert.InvalidArgument("args must contain strings")
		}
		argv = append(argv, arg)
	}
	return argv, nil
}

func detailedPlanRequested(argv []string) bool {
	enabled := false
	for _, arg := range argv {
		switch arg {
		case "-detailed-exitcode", "-detailed-exitcode=true":
			enabled = true
		case "-detailed-exitcode=false":
			enabled = false
		}
	}
	return enabled
}

// atmosFlags emits one argv entry per value; values never undergo shell parsing.
// Bare names use long flags; an explicit dash prefix preserves native tool flags.
func (s *binding) atmosFlags(path []string, flags *starlark.Dict) ([]string, error) {
	if flags == nil {
		return nil, nil
	}
	entries := make(map[string]starlark.Value, flags.Len())
	for _, item := range flags.Items() {
		key, ok := starlark.AsString(item[0])
		if !ok {
			return nil, convert.InvalidArgument("flags must have string keys")
		}
		entries[key] = item[1]
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var argv []string
	seen := make(map[string]bool)
	for _, key := range keys {
		flag, err := s.atmosFlagName(path, key)
		if err != nil {
			return nil, err
		}
		if seen[flag] {
			return nil, convert.InvalidArgument("duplicate flag %s", flag)
		}
		seen[flag] = true
		values, err := atmosFlagArgs(flag, entries[key])
		if err != nil {
			return nil, err
		}
		argv = append(argv, values...)
	}
	return argv, nil
}

func (s *binding) atmosFlagName(path []string, key string) (string, error) {
	name := strings.TrimLeft(key, atmosFlagPrefix)
	if name == "" || strings.ContainsAny(key, "= \t\r\n") {
		return "", convert.InvalidArgument("flags must have nonempty flag-name keys")
	}
	if (path[0] == "terraform" || path[0] == "helm") && (name == "stack" || name == "s") {
		return "", convert.InvalidArgument("use the stack argument instead of a stack flag")
	}
	return s.catalog.FlagName(path, key), nil
}

func atmosFlagArgs(flag string, value starlark.Value) ([]string, error) {
	values := []starlark.Value{value}
	if list, ok := value.(*starlark.List); ok {
		values, _ = convert.Sequence(list)
	}
	var argv []string
	for _, v := range values {
		if v == starlark.True {
			argv = append(argv, flag)
			continue
		}
		text, err := atmosFlagValue(v)
		if err != nil {
			return nil, err
		}
		argv = append(argv, flag+"="+text)
	}
	return argv, nil
}

func atmosFlagValue(value starlark.Value) (string, error) {
	switch value := value.(type) {
	case starlark.String:
		return string(value), nil
	case starlark.Bool:
		if value {
			return "true", nil
		}
		return "false", nil
	case starlark.Int:
		return value.String(), nil
	default:
		return "", convert.InvalidArgument("flag values must be strings, booleans, integers, or lists of these")
	}
}

// atmosCommand prefixes the selected built-in command to literal CLI arguments.
// Options are keyword-only, so arbitrary subcommands and positionals remain usable.
func (s *binding) atmosCommand(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var flags *starlark.Dict
	var extra starlark.Value = starlark.Tuple{}
	opts := Options{Dir: s.directory, Output: streamOutput, Check: true}
	if err := starlark.UnpackArgs(b.Name(), nil, kwargs, "flags?", &flags, "args?", &extra,
		workingDirectoryArg, &opts.Dir, environmentArg, &opts.Env, outputArg, &opts.Output, checkArg, &opts.Check); err != nil {
		return nil, err
	}
	positionals, err := atmosExtraArgs(args)
	if err != nil {
		return nil, err
	}
	name := strings.TrimPrefix(b.Name(), "atmos.")
	extraArgs, err := atmosExtraArgs(extra)
	if err != nil {
		return nil, err
	}
	commandPath := append(append([]string{name}, positionals...), extraArgs...)
	flagArgs, err := s.atmosFlags(commandPath, flags)
	if err != nil {
		return nil, err
	}
	argv := append([]string{name}, positionals...)
	argv = append(argv, flagArgs...)
	argv = append(argv, extraArgs...)
	return s.run(thread, argv, opts, false)
}
