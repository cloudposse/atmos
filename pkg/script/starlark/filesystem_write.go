package starlark

import (
	"errors"
	"os"

	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
)

// writeFile atomically writes the exact bytes of a Starlark string, preserving an
// existing regular file's permissions. New files default to 0644. It rejects
// symlink destinations rather than following them. Parent directories must exist.
func (s *session) writeFile(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path, content string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path, "content", &content); err != nil {
		return nil, err
	}
	if err := threadContext(t).Err(); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, invalidArg("fs.write_file: path must not be empty")
	}
	dest := s.filesystemPath(t, path)
	mode := os.FileMode(defaultWriteMode)
	info, err := os.Lstat(dest)
	if err == nil {
		if !info.Mode().IsRegular() {
			return nil, invalidArg("fs.write_file: destination must be a regular file")
		}
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return starlark.None, s.engine.writeFile(dest, []byte(content), mode)
}

// WithWriteFile supplies the atomic file-write boundary for embedded hosts and tests.
func WithWriteFile(write func(string, []byte, os.FileMode) error) Option {
	defer perf.Track(nil, "starlark.WithWriteFile")()
	return func(e *Engine) { e.writeFile = write }
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	return (&filesystem.OSFileSystem{}).WriteFileAtomic(path, data, mode)
}

const defaultWriteMode = 0o644
