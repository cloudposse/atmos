package starlark

import (
	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Reads a file. Absolute paths are used as-is; relative paths, including parent-directory
// segments, resolve against the working directory, matching load().
func (s *session) readFile(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	ctx := threadContext(thread)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	contents, err := s.engine.readFile(s.filesystemPath(path))
	if err != nil {
		return nil, failWith(errUtils.ErrStarlark, err, "cannot read file: %s", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return starlark.String(contents), nil
}
