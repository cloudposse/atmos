package starlark

import (
	"os"
	"path/filepath"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
)

// WithFileSystem supplies read-only filesystem metadata operations.
func WithFileSystem(fs api.FileSystem) Option {
	defer perf.Track(nil, "starlark.WithFileSystem")()
	return func(e *Engine) { e.filesystem = fs }
}

func (s *session) filesystemModule() starlark.Value {
	return module("fs", starlark.StringDict{
		"read_file":  starlark.NewBuiltin("fs.read_file", s.readFile),
		"write_file": starlark.NewBuiltin("fs.write_file", s.writeFile),
		"mkdir":      starlark.NewBuiltin("fs.mkdir", s.mkdir),
		"mkdtemp":    starlark.NewBuiltin("fs.mkdtemp", s.mkdtemp),
		"remove":     starlark.NewBuiltin("fs.remove", s.remove),
		"glob":       starlark.NewBuiltin("fs.glob", s.glob),
		"stat":       starlark.NewBuiltin("fs.stat", s.stat),
		"exists":     starlark.NewBuiltin("fs.exists", s.exists),
		"readlink":   starlark.NewBuiltin("fs.readlink", s.readlink),
		"resolve":    starlark.NewBuiltin("fs.resolve", s.resolve),
	})
}

// filesystemPath resolves a script-supplied path: a leading `~` expands to the home directory,
// relative paths join onto the working directory, and the result is cleaned.
func (s *session) filesystemPath(t *starlark.Thread, path string) string {
	path = filesystem.ExpandHome(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.executionScope(t).dir, path)
	}
	return filepath.Clean(path)
}

func (s *session) glob(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var pattern string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "pattern", &pattern); err != nil {
		return nil, err
	}
	// A leading `~` expands to an absolute path, so matches are reported as absolute paths.
	absolutePattern := filepath.IsAbs(filesystem.ExpandHome(pattern))
	matches, err := s.engine.filesystem.Glob(threadContext(t), s.filesystemPath(t, pattern))
	if err != nil {
		return nil, err
	}
	values := make([]starlark.Value, 0, len(matches))
	for _, path := range matches {
		if !absolutePattern {
			path, err = filepath.Rel(s.executionScope(t).dir, path)
			if err != nil {
				return nil, err
			}
		}
		values = append(values, starlark.String(filepath.ToSlash(path)))
	}
	return starlark.NewList(values), nil
}

func (s *session) stat(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	follow := true
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path, "follow_symlinks?", &follow); err != nil {
		return nil, err
	}
	info, err := s.engine.filesystem.Stat(threadContext(t), s.filesystemPath(t, path), follow)
	if err != nil {
		return nil, err
	}
	return starlarkstruct.FromStringDict(starlark.String("file_info"), starlark.StringDict{
		"size": starlark.MakeInt64(info.Size), "is_file": starlark.Bool(info.IsFile),
		"is_dir": starlark.Bool(info.IsDir), "is_symlink": starlark.Bool(info.IsSymlink),
	}), nil
}

func (s *session) exists(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	exists, err := s.engine.filesystem.Exists(threadContext(t), s.filesystemPath(t, path))
	return starlark.Bool(exists), err
}

func (s *session) readlink(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	target, err := s.engine.filesystem.Readlink(threadContext(t), s.filesystemPath(t, path))
	return starlark.String(target), err
}

// resolve returns the absolute path for a script-supplied path using the same rules as every
// other fs function. It does not touch the filesystem, so the path does not need to exist.
func (s *session) resolve(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, invalidArg("fs.resolve: path must not be empty")
	}
	if err := threadContext(t).Err(); err != nil {
		return nil, err
	}
	return starlark.String(s.filesystemPath(t, path)), nil
}

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
