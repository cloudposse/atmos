package adoption

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
	y "github.com/cloudposse/atmos/pkg/yaml"
)

// File holds the original and proposed bytes for one atomic configuration edit.
type File struct {
	Path          string
	Before, After []byte
	mode          os.FileMode
}

// OpenFile prepares an existing or new regular file for editing.
func OpenFile(path string) (*File, error) {
	defer perf.Track(nil, "adoption.OpenFile")()

	b, mode, err := ReadRegular(path)
	return &File{Path: path, Before: b, After: b, mode: mode}, err
}

// Set edits a typed field in the proposed YAML document.
func (f *File) Set(key, value, kind string) error {
	defer perf.Track(nil, "adoption.File.Set")()

	b, err := y.SetWithType(f.After, key, value, kind)
	if err == nil {
		f.After = b
	}
	return err
}

// Write atomically persists changes and preserves the original file mode.
func (f *File) Write() error {
	defer perf.Track(nil, "adoption.File.Write")()

	if bytes.Equal(f.After, f.Before) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), configDirMode); err != nil {
		return err
	}
	return filesystem.NewOSFileSystem().WriteFileAtomic(f.Path, f.After, f.mode)
}
