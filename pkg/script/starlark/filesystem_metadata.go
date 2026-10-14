package starlark

import (
	"path/filepath"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/cloudposse/atmos/pkg/automation"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
)

// WithFileSystem supplies read-only filesystem metadata operations.
func WithFileSystem(fs automation.FileSystem) Option {
	defer perf.Track(nil, "starlark.WithFileSystem")()
	return func(e *Engine) { e.filesystem = fs }
}

func (s *session) filesystemModule() starlark.Value {
	return module("fs", starlark.StringDict{
		"read_file": starlark.NewBuiltin("fs.read_file", s.readFile),
		"glob":      starlark.NewBuiltin("fs.glob", s.glob),
		"stat":      starlark.NewBuiltin("fs.stat", s.stat),
		"exists":    starlark.NewBuiltin("fs.exists", s.exists),
		"readlink":  starlark.NewBuiltin("fs.readlink", s.readlink),
		"resolve":   starlark.NewBuiltin("fs.resolve", s.resolve),
	})
}

// filesystemPath resolves a script-supplied path: a leading `~` expands to the home directory,
// relative paths join onto the working directory, and the result is cleaned.
func (s *session) filesystemPath(path string) string {
	path = filesystem.ExpandHome(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.spec.WorkingDirectory, path)
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
	matches, err := s.engine.filesystem.Glob(threadContext(t), s.filesystemPath(pattern))
	if err != nil {
		return nil, err
	}
	values := make([]starlark.Value, 0, len(matches))
	for _, path := range matches {
		if !absolutePattern {
			path, err = filepath.Rel(s.spec.WorkingDirectory, path)
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
	info, err := s.engine.filesystem.Stat(threadContext(t), s.filesystemPath(path), follow)
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
	exists, err := s.engine.filesystem.Exists(threadContext(t), s.filesystemPath(path))
	return starlark.Bool(exists), err
}

func (s *session) readlink(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	target, err := s.engine.filesystem.Readlink(threadContext(t), s.filesystemPath(path))
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
	return starlark.String(s.filesystemPath(path)), nil
}
