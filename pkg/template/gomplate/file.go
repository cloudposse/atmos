package gomplate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hairyhenderson/gomplate/v4/conv"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	nativeFileMode        = 0o644
	nativeWindowsFileMode = 0o666
	nativeDirectoryMode   = 0o755
)

// Native file functions avoid Gomplate's fallback filesystem binding a non-C: working
// directory to the C: volume on Windows. Policy admission wraps these methods as usual.
type nativeFileFuncs struct{}

func (nativeFileFuncs) Read(path any) (string, error) {
	defer perf.Track(nil, "gomplate.file.Read")()

	content, err := os.ReadFile(conv.ToString(path))
	return string(content), err
}

func (nativeFileFuncs) Stat(path any) (os.FileInfo, error) {
	defer perf.Track(nil, "gomplate.file.Stat")()

	return os.Stat(conv.ToString(path))
}

func (f nativeFileFuncs) Exists(path any) bool {
	defer perf.Track(nil, "gomplate.file.Exists")()

	_, err := f.Stat(path)
	return err == nil
}

func (f nativeFileFuncs) IsDir(path any) bool {
	defer perf.Track(nil, "gomplate.file.IsDir")()

	info, err := f.Stat(path)
	return err == nil && info.IsDir()
}

func (nativeFileFuncs) ReadDir(path any) ([]string, error) {
	defer perf.Track(nil, "gomplate.file.ReadDir")()

	entries, err := os.ReadDir(conv.ToString(path))
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names, nil
}

func (nativeFileFuncs) Walk(path any) ([]string, error) {
	defer perf.Track(nil, "gomplate.file.Walk")()

	root := conv.ToString(path)
	files := make([]string, 0)
	// Stat follows a root symlink, matching Gomplate's fs.WalkDir behavior.
	info, err := os.Stat(root)
	if err != nil {
		return files, err
	}
	files = append(files, root)
	if !info.IsDir() {
		return files, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return files, err
	}
	// Native filepath traversal preserves Windows UNC volumes. WalkDir does not
	// follow child symlinks, including children immediately beneath the root.
	for _, entry := range entries {
		err := filepath.WalkDir(filepath.Join(root, entry.Name()), func(path string, _ fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return files, err
		}
	}
	return files, nil
}

func (nativeFileFuncs) Write(path, value any) (string, error) {
	defer perf.Track(nil, "gomplate.file.Write")()

	filename := conv.ToString(path)
	if err := assertFilePathInWD(filename); err != nil {
		return "", fmt.Errorf("failed to open %s: %w", filename, err)
	}
	info, err := os.Stat(filename)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("failed to stat %s: %w", filename, err)
	}
	mode := os.FileMode(nativeFileMode)
	if runtime.GOOS == "windows" {
		mode = nativeWindowsFileMode
	}
	if info != nil {
		mode = info.Mode()
	}
	if err := os.MkdirAll(filepath.Dir(filename), nativeDirectoryMode); err != nil {
		return "", fmt.Errorf("failed to make dirs for %s: %w", filename, err)
	}
	if err := os.WriteFile(filename, fileContent(value), mode); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", filename, err)
	}
	return "", nil
}

func fileContent(value any) []byte {
	switch value := value.(type) {
	case []byte:
		return value
	case interface{ Bytes() []byte }:
		return value.Bytes()
	default:
		return []byte(conv.ToString(value))
	}
}

func assertFilePathInWD(filename string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(filename)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(wd, absolute)
	if err != nil {
		return err
	}
	// Preserve Gomplate's lexical restriction, including names beginning with "..".
	if strings.HasPrefix(relative, "..") {
		return fmt.Errorf("%w: path %s not contained by working directory %s (rel: %s)", errUtils.ErrWriteFile, filename, wd, relative)
	}
	return nil
}
