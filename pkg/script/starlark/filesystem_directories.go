package starlark

import (
	"os"
	"path/filepath"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
)

const defaultDirectoryMode = 0o700

func (s *session) mkdir(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	parents, mode := false, defaultDirectoryMode
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path, "parents?", &parents, "mode?", &mode); err != nil {
		return nil, err
	}
	if err := threadContext(t).Err(); err != nil {
		return nil, err
	}
	if path == "" || mode < 0 || mode > maxWriteMode {
		return nil, invalidArg("fs.mkdir: nonempty path and mode between 0o000 and 0o777 required")
	}
	create := os.Mkdir
	if parents {
		create = os.MkdirAll
	}
	if err := create(s.filesystemPath(t, path), os.FileMode(mode)); err != nil {
		return nil, failWith(errUtils.ErrStarlark, err, "cannot create directory: %s", err)
	}
	return starlark.None, nil
}

func (s *session) mkdtemp(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	prefix, directory := "atmos-", ""
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "prefix?", &prefix, "dir?", &directory); err != nil {
		return nil, err
	}
	if err := threadContext(t).Err(); err != nil {
		return nil, err
	}
	if directory != "" {
		directory = s.filesystemPath(t, directory)
	}
	path, err := os.MkdirTemp(directory, prefix)
	if err != nil {
		return nil, failWith(errUtils.ErrStarlark, err, "cannot create temporary directory: %s", err)
	}
	return starlark.String(path), nil
}

// remove does not follow a final symlink. Recursive deletion must be explicit;
// rejecting empty paths and filesystem roots protects against missing arguments.
func (s *session) remove(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	recursive := false
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path, "recursive?", &recursive); err != nil {
		return nil, err
	}
	if err := threadContext(t).Err(); err != nil {
		return nil, err
	}
	target := s.filesystemPath(t, path)
	if path == "" || filepath.Dir(target) == target {
		return nil, invalidArg("fs.remove: path must not be empty or a filesystem root")
	}
	remove := os.Remove
	if recursive {
		remove = os.RemoveAll
	}
	if err := remove(target); err != nil {
		return nil, failWith(errUtils.ErrStarlark, err, "cannot remove path: %s", err)
	}
	return starlark.None, nil
}
