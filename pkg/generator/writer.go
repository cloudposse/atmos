package generator

import (
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// defaultFileMode is the default permission mode for generated files.
const defaultFileMode os.FileMode = 0o600

// Writer handles file output for generators.
type Writer interface {
	// WriteJSON writes content as JSON to the specified directory and filename.
	WriteJSON(dir, filename string, data map[string]any) error
	// WriteHCL writes content as HCL to the specified directory and filename.
	WriteHCL(dir, filename string, data map[string]any) error
}

// FileWriter is the production implementation that writes to the filesystem.
type FileWriter struct {
	fileMode os.FileMode
}

// NewFileWriter creates a new file writer with optional configuration.
func NewFileWriter(opts ...WriterOption) *FileWriter {
	defer perf.Track(nil, "generator.NewFileWriter")()

	w := &FileWriter{
		fileMode: defaultFileMode,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// WriteJSON writes content as JSON to the specified directory and filename.
func (w *FileWriter) WriteJSON(dir, filename string, data map[string]any) error {
	defer perf.Track(nil, "generator.FileWriter.WriteJSON")()

	path := filepath.Join(dir, filename)
	return u.WriteToFileAsJSON(path, data, w.fileMode)
}

// WriteHCL writes content as HCL to the specified directory and filename.
func (w *FileWriter) WriteHCL(dir, filename string, data map[string]any) error {
	defer perf.Track(nil, "generator.FileWriter.WriteHCL")()

	path := filepath.Join(dir, filename)
	return u.WriteToFileAsHcl(path, data, w.fileMode)
}

// MockWriter is a test implementation that captures written content.
type MockWriter struct {
	// Written maps file paths to their written content.
	Written map[string]map[string]any
	// WriteErr if set, will be returned by Write operations.
	WriteErr error
}

// NewMockWriter creates a new mock writer for testing.
func NewMockWriter() *MockWriter {
	defer perf.Track(nil, "generator.NewMockWriter")()

	return &MockWriter{
		Written: make(map[string]map[string]any),
	}
}

// WriteJSON captures the content without writing to disk.
func (w *MockWriter) WriteJSON(dir, filename string, data map[string]any) error {
	if w.WriteErr != nil {
		return w.WriteErr
	}
	path := filepath.Join(dir, filename)
	w.Written[path] = data
	return nil
}

// WriteHCL captures the content without writing to disk.
func (w *MockWriter) WriteHCL(dir, filename string, data map[string]any) error {
	if w.WriteErr != nil {
		return w.WriteErr
	}
	path := filepath.Join(dir, filename)
	w.Written[path] = data
	return nil
}

// GetWritten returns the content written to a specific path.
func (w *MockWriter) GetWritten(dir, filename string) (map[string]any, bool) {
	path := filepath.Join(dir, filename)
	data, ok := w.Written[path]
	return data, ok
}

// Clear resets the mock writer state.
func (w *MockWriter) Clear() {
	w.Written = make(map[string]map[string]any)
	w.WriteErr = nil
}
