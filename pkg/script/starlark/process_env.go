package starlark

import (
	"runtime"
	"strings"

	"go.starlark.net/starlark"
)

// getenv reads the same environment as exec.run, without changing the script env dictionary.
func (s *session) getenv(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var fallback starlark.Value = starlark.None
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "name", &name, "default?", &fallback); err != nil {
		return nil, err
	}
	if name == "" || strings.ContainsAny(name, "=\x00") {
		return nil, invalidArg("os.getenv: name must be a nonempty environment variable name")
	}
	if err := threadContext(thread).Err(); err != nil {
		return nil, err
	}
	env := s.tools.Environment(s.osEnvironment(s.spec.ProcessEnv))
	for i := len(env) - 1; i >= 0; i-- {
		key, value, ok := strings.Cut(env[i], "=")
		if ok && (key == name || (runtime.GOOS == "windows" && strings.EqualFold(key, name))) {
			return starlark.String(value), nil
		}
	}
	return fallback, nil
}
