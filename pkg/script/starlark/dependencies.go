package starlark

import (
	"context"
	"strings"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
)

func (s *session) installTools(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, version string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "name", &name, "version", &version); err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
		return nil, invalidArg("tool name and version must be nonempty")
	}
	// The parent waits while branches run. Restrict environment changes to that
	// parent so all parallel branches inherit the same pinned tools.
	if thread.Local(outputKey) != nil {
		return nil, invalidArg("declare dependencies.tools before starting parallel tasks")
	}
	if previous, ok := s.tools[name]; ok {
		if previous != version {
			return nil, invalidArg("tool %q is already pinned to %q in this script", name, previous)
		}
		return starlark.None, nil
	}
	return s.provisionTool(threadContext(thread), name, version)
}

func (s *session) provisionTool(ctx context.Context, name, version string) (starlark.Value, error) {
	if s.spec.InstallTools == nil {
		return nil, fail(errUtils.ErrStarlark, "tool installation is unavailable in this execution context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dirs, err := s.spec.InstallTools(ctx, map[string]string{name: version})
	if err != nil {
		return nil, failWith(errUtils.ErrStarlark, err, "install dependency %s@%s: %s", name, version, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.tools == nil {
		s.tools = make(map[string]string)
	}
	s.tools[name] = version
	s.toolDirs = append(s.toolDirs, dirs...)
	return starlark.None, nil
}
