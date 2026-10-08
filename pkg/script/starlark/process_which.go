package starlark

import (
	"go.starlark.net/starlark"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/process"
)

// which resolves a program name the way exec.run would start it: against the PATH of the
// effective execution environment, including directories added by dependencies.tools, with
// names containing a separator resolved relative to the working directory. A name that does not
// resolve to an executable returns None rather than failing, so scripts can branch on it.
func (s *session) which(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, invalidArg("exec.which: name must not be empty")
	}
	if err := threadContext(thread).Err(); err != nil {
		return nil, err
	}
	env, err := processEnv(s.spec.ProcessEnv, nil)
	if err != nil {
		return nil, err
	}
	path, err := process.LookPath(s.spec.WorkingDirectory, s.tools.Environment(env), name)
	if err != nil {
		log.Debug("exec.which did not resolve the program", "name", name, "error", err)
		return starlark.None, nil
	}
	return starlark.String(path), nil
}
