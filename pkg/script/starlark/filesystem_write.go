package starlark

import (
	"os"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
)

const (
	defaultWriteMode = 0o600
	maxWriteMode     = 0o777
)

func (s *session) writeFile(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path, content string
	overwrite, mode := false, defaultWriteMode
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path, "content", &content, "overwrite?", &overwrite, "mode?", &mode); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, invalidArg("fs.write_file: path must not be empty")
	}
	if mode < 0 || mode > maxWriteMode {
		return nil, invalidArg("fs.write_file: mode must be between 0o000 and 0o777")
	}
	err := filesystem.WriteFile(threadContext(thread), s.filesystemPath(path), []byte(content), filesystem.WriteOptions{Mode: os.FileMode(mode), Overwrite: overwrite})
	if err != nil {
		return nil, failWith(errUtils.ErrStarlark, err, "cannot write file: %s", err)
	}
	return starlark.None, nil
}
