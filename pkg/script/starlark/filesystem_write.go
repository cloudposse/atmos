package starlark

import (
	"errors"
	"os"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
)

// writeFile creates a file exclusively or atomically replaces it with overwrite=True,
// preserving existing permissions. New files default to 0600. It rejects
// symlink destinations rather than following them. Parent directories must exist.
func (s *session) writeFile(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path, content string
	overwrite, requestedMode := false, defaultWriteMode
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path, "content", &content, "overwrite?", &overwrite, "mode?", &requestedMode); err != nil {
		return nil, err
	}
	if err := threadContext(t).Err(); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, invalidArg("fs.write_file: path must not be empty")
	}
	if requestedMode < 0 || requestedMode > maxWriteMode {
		return nil, invalidArg("fs.write_file: mode must be between 0o000 and 0o777")
	}
	dest := s.filesystemPath(t, path)
	if !overwrite {
		err := filesystem.WriteFile(threadContext(t), dest, []byte(content), filesystem.WriteOptions{Mode: os.FileMode(requestedMode)})
		if err != nil {
			return nil, failWith(errUtils.ErrStarlark, err, "cannot write file: %s", err)
		}
		return starlark.None, nil
	}
	return starlark.None, s.replaceFile(dest, []byte(content), os.FileMode(requestedMode))
}

func (s *session) replaceFile(dest string, content []byte, mode os.FileMode) error {
	info, err := os.Lstat(dest)
	if err == nil {
		if !info.Mode().IsRegular() {
			return invalidArg("fs.write_file: destination must be a regular file")
		}
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return s.engine.writeFile(dest, content, mode)
}

// WithWriteFile supplies the atomic file-write boundary for embedded hosts and tests.
func WithWriteFile(write func(string, []byte, os.FileMode) error) Option {
	defer perf.Track(nil, "starlark.WithWriteFile")()
	return func(e *Engine) { e.writeFile = write }
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	return (&filesystem.OSFileSystem{}).WriteFileAtomic(path, data, mode)
}

const (
	defaultWriteMode = 0o600
	maxWriteMode     = 0o777
)
