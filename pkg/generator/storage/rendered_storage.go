package storage

import (
	"errors"
	"os"
	"path/filepath"

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

	cleanPath := filepath.Clean(filePath)
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
