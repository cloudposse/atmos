package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RenderedBaseStorage provides base-file storage for a 3-way merge whose base
// comes from a pristine template re-render rather than the target's own git
// history (see GitBaseStorage). It's a plain directory read: root is expected
// to already contain a fully-rendered copy of the template at whichever ref
// produced what's currently on disk.
type RenderedBaseStorage struct {
	root string
}

// NewRenderedBaseStorage creates base storage backed by an already-rendered
// directory tree at root.
func NewRenderedBaseStorage(root string) *RenderedBaseStorage {
	defer perf.Track(nil, "storage.NewRenderedBaseStorage")()

	return &RenderedBaseStorage{root: root}
}

// LoadBase retrieves the content of a file from the pristine render.
//
// Note: filePath should be relative to the render root, matching
// GitBaseStorage.LoadBase's contract:
//   - File content as string if the file exists in the render
//   - Empty string and (false, nil) if the file doesn't exist in the render
//   - Error if the render root itself can't be read
func (s *RenderedBaseStorage) LoadBase(filePath string) (string, bool, error) {
	defer perf.Track(nil, "storage.RenderedBaseStorage.LoadBase")()

	// engine.Processor's own caller (determineBaseContent) normally passes a
	// path already made relative to the target directory, but it falls back
	// to the scaffold's raw, un-rendered file.Path when that computation
	// fails (see merge_update.go) -- a path this method has no other
	// opportunity to validate before it reaches os.ReadFile below.
	// filepath.Clean alone does not strip a leading ".." (it only collapses
	// redundant separators/segments), so an unvalidated "../../etc/passwd"
	// would otherwise let filepath.Join walk fullPath outside s.root
	// entirely. Reject that here rather than relying on every caller to
	// have already sanitized filePath.
	cleanPath := filepath.Clean(filePath)
	if filepath.IsAbs(cleanPath) || IsRootedOnAnyOS(cleanPath) || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return "", false, errUtils.Build(errUtils.ErrPathTraversal).
			WithExplanationf("Rendered base path escapes the render root: `%s`", filePath).
			WithContext("file_path", filePath).
			WithContext("render_root", s.root).
			WithExitCode(2).
			Err()
	}
	fullPath := filepath.Join(s.root, cleanPath)

	content, err := os.ReadFile(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// File doesn't exist in the pristine render -- this is not an
			// error, just means no base version (e.g. a user-added file).
			return "", false, nil
		}
		return "", false, errUtils.Build(errUtils.ErrReadFile).
			WithCause(err).
			WithExplanationf("Failed to read rendered base file: `%s`", filePath).
			WithContext("file_path", filePath).
			WithContext("render_root", s.root).
			WithExitCode(2).
			Err()
	}

	return string(content), true, nil
}

// IsRootedOnAnyOS reports whether path is rooted under *any* OS's
// convention, not just the OS this binary is running on. filepath.IsAbs is
// insufficient here: on Windows it doesn't consider "/etc/passwd" absolute
// (Windows requires a drive letter or UNC prefix), and on Unix it doesn't
// consider "C:\Windows\System32" or "\etc\passwd" absolute. A path-traversal
// guard fed an untrusted scaffold manifest (which may be authored on a
// different OS than whatever runs the guard) must reject a path rooted by
// either convention regardless of runtime GOOS.
func IsRootedOnAnyOS(path string) bool {
	if path == "" {
		return false
	}
	if path[0] == '/' || path[0] == '\\' {
		return true
	}
	// Windows drive-letter prefix, e.g. "C:\Windows" or "C:/Windows".
	if len(path) >= 2 && path[1] == ':' && isASCIILetter(path[0]) {
		return true
	}
	return false
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
