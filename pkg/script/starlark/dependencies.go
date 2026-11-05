package starlark

import (
	"errors"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
)

func (s *session) installTools(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, version string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "name", &name, "version", &version); err != nil {
		return nil, err
	}
	// The parent waits while branches run. Restrict environment changes to that
	// parent so all parallel branches inherit the same pinned tools.
	if thread.Local(outputKey) != nil {
		return nil, invalidArg("declare dependencies.tools before starting parallel tasks")
	}
	err := s.tools.Install(threadContext(thread), name, version)
	if errors.Is(err, errUtils.ErrScriptInvalidArgument) {
		return nil, invalidArg("%s", err)
	}
	if err != nil {
		return nil, err
	}
	return starlark.None, nil
}
