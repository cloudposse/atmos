package automation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// FileSystem supplies read-only filesystem operations to embedded interpreters.
// Paths are absolute when called by an interpreter. Implementations must be safe for concurrent use.
type FileSystem interface {
	Glob(context.Context, string) ([]string, error)
	Stat(context.Context, string, bool) (FileInfo, error)
	Exists(context.Context, string) (bool, error)
	Readlink(context.Context, string) (string, error)
}

// FileInfo describes a file without reading its contents. Size is measured in bytes.
type FileInfo struct {
	Size                     int64
	IsFile, IsDir, IsSymlink bool
}

// LocalFileSystem implements read-only operations using the operating system.
type LocalFileSystem struct{}

// Glob returns sorted matches using filepath glob syntax. It does not support recursive ** matching.
func (LocalFileSystem) Glob(ctx context.Context, pattern string) ([]string, error) {
	defer perf.Track(nil, "automation.LocalFileSystem.Glob")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: glob %q: %w", errUtils.ErrAutomation, pattern, err)
	}
	sort.Strings(matches)
	return matches, ctx.Err()
}

// Stat returns file metadata, following symlinks unless followSymlinks is false.
func (LocalFileSystem) Stat(ctx context.Context, path string, followSymlinks bool) (FileInfo, error) {
	defer perf.Track(nil, "automation.LocalFileSystem.Stat")()
	if err := ctx.Err(); err != nil {
		return FileInfo{}, err
	}
	stat := os.Stat
	if !followSymlinks {
		stat = os.Lstat
	}
	info, err := stat(path)
	if err != nil {
		return FileInfo{}, fmt.Errorf("%w: stat %q: %w", errUtils.ErrAutomation, path, err)
	}
	return FileInfo{Size: info.Size(), IsFile: info.Mode().IsRegular(), IsDir: info.IsDir(), IsSymlink: info.Mode()&os.ModeSymlink != 0}, ctx.Err()
}

// Exists follows symlinks. Missing paths and dangling links return false; other failures are errors.
func (fs LocalFileSystem) Exists(ctx context.Context, path string) (bool, error) {
	defer perf.Track(nil, "automation.LocalFileSystem.Exists")()
	_, err := fs.Stat(ctx, path, true)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Readlink returns the stored symlink target without resolving it or requiring it to exist.
func (LocalFileSystem) Readlink(ctx context.Context, path string) (string, error) {
	defer perf.Track(nil, "automation.LocalFileSystem.Readlink")()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	target, err := os.Readlink(path)
	if err != nil {
		return "", fmt.Errorf("%w: readlink %q: %w", errUtils.ErrAutomation, path, err)
	}
	return target, ctx.Err()
}
