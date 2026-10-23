package sandbox

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/filesystem"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

type fileState struct {
	Data   []byte
	Mode   os.FileMode
	Exists bool
}

func readState(path string) (fileState, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileState{}, nil
	}
	if err != nil {
		return fileState{}, err
	}
	if info.IsDir() {
		return fileState{}, fmt.Errorf("%w: cannot edit directory or submodule %s", ErrFileType, path)
	}
	s := fileState{Mode: info.Mode(), Exists: true}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		s.Data = []byte(target)
		return s, err
	}
	if !info.Mode().IsRegular() {
		return fileState{}, fmt.Errorf("%w: %s", ErrFileType, path)
	}
	s.Data, err = os.ReadFile(path)
	return s, err
}

func equalState(a, b fileState) bool {
	return a.Exists == b.Exists && a.Mode == b.Mode && bytes.Equal(a.Data, b.Data)
}

// Never follow a symlink while writing a formatter result.
func writeState(path string, state fileState) error {
	if !state.Exists {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), checkoutDirMode); err != nil {
		return err
	}
	if state.Mode&os.ModeSymlink != 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return os.Symlink(string(state.Data), path)
	}
	return (&filesystem.OSFileSystem{}).WriteFileAtomic(path, state.Data, state.Mode.Perm())
}

func safePath(root, name string) (string, error) {
	if !filepath.IsLocal(name) || filepath.Clean(name) == "." {
		return "", fmt.Errorf("%w: %q", ErrUnsafePath, name)
	}
	for _, part := range strings.Split(filepath.ToSlash(filepath.Clean(name)), "/") {
		if strings.EqualFold(part, ".git") {
			return "", fmt.Errorf("%w: Git metadata %q", ErrUnsafePath, name)
		}
	}
	// Match the absolute, native spelling returned by path validation.
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path, err := atmosgit.ValidateRepoRelativePath(root, filepath.FromSlash(name))
	if err != nil {
		return "", err
	}
	for parent := filepath.Dir(path); parent != root && parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf("%w: non-directory parent %s", ErrUnsafePath, parent)
		}
	}
	return path, nil
}
