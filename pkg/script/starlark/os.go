package starlark

import (
	"runtime"
	"strings"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/script"
)

// osEnvironment applies invocation-local mutations after component and step inputs.
// Only the main thread may mutate osEnv, and it waits while parallel tasks run.
func (s *session) osEnvironment(thread *starlark.Thread, base []string) []string {
	overrides := s.osEnv
	if scope, ok := threadContext(thread).Value(sandboxScopeKey{}).(*sandboxScope); ok {
		overrides = scope.osEnv
	}
	if len(overrides) == 0 {
		return base
	}
	env := make([]string, 0, len(base))
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if _, overridden := overrides[environmentKey(name)]; !overridden {
			env = append(env, entry)
		}
	}
	return script.ProcessEnvironment(env, overrides)
}

func environmentKey(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(name)
	}
	return name
}

func (s *session) environ(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs(b.Name(), args, kwargs); err != nil {
		return nil, err
	}
	if err := threadContext(thread).Err(); err != nil {
		return nil, err
	}
	values := make(map[string]string)
	for _, entry := range s.tools.Environment(s.osEnvironment(thread, s.executionScope(thread).env)) {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name != "" {
			values[environmentKey(name)] = value
		}
	}
	return stringDict(values), nil
}

func (s *session) setenv(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, value string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "name", &name, "value", &value); err != nil {
		return nil, err
	}
	if name == "" || strings.ContainsAny(name, "=\x00") {
		return nil, invalidArg("os.setenv: name must be a nonempty environment variable name")
	}
	if strings.ContainsRune(value, '\x00') {
		return nil, invalidArg("os.setenv: value must not contain null bytes")
	}
	if err := osMutationAllowed(thread, b.Name()); err != nil {
		return nil, err
	}
	if scope, ok := threadContext(thread).Value(sandboxScopeKey{}).(*sandboxScope); ok {
		if scope.osEnv == nil {
			scope.osEnv = make(map[string]string)
		}
		scope.osEnv[environmentKey(name)] = value
		return starlark.None, nil
	}
	if s.osEnv == nil {
		s.osEnv = make(map[string]string)
	}
	s.osEnv[environmentKey(name)] = value
	return starlark.None, nil
}

func (s *session) getcwd(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs(b.Name(), args, kwargs); err != nil {
		return nil, err
	}
	if err := threadContext(thread).Err(); err != nil {
		return nil, err
	}
	return starlark.String(s.executionScope(thread).dir), nil
}

func (s *session) chdir(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	if path == "" || strings.ContainsRune(path, '\x00') {
		return nil, invalidArg("os.chdir: path must be nonempty and contain no null bytes")
	}
	if err := osMutationAllowed(thread, b.Name()); err != nil {
		return nil, err
	}
	dir := s.filesystemPath(thread, path)
	info, err := s.engine.filesystem.Stat(threadContext(thread), dir, true)
	if err != nil {
		return nil, failWith(errUtils.ErrStarlark, err, "os.chdir: cannot access directory: %s", err)
	}
	if !info.IsDir {
		return nil, invalidArg("os.chdir: path must be a directory")
	}
	if err := threadContext(thread).Err(); err != nil {
		return nil, err
	}
	if scope, ok := threadContext(thread).Value(sandboxScopeKey{}).(*sandboxScope); ok {
		scope.dir = dir
		return starlark.None, nil
	}
	s.spec.WorkingDirectory = dir
	s.spec.AtmosWorkingDirectory = dir
	return starlark.None, nil
}

func osMutationAllowed(thread *starlark.Thread, name string) error {
	if thread.Local(outputKey) != nil {
		return invalidArg("%s is available only in the main thread; use per-call env or working_directory inside parallel tasks", name)
	}
	return threadContext(thread).Err()
}
