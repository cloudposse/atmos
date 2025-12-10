package output

import (
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/list/format"
)

// Manager routes output to data or ui layer based on format.
type Manager struct {
	format format.Format
}

// New creates an output manager for the specified format.
func New(fmt format.Format) *Manager {
	return &Manager{format: fmt}
}

// Write routes content to the appropriate output stream.
// All list output goes to data.Write() (stdout) for pipeability.
// List output includes JSON, YAML, CSV, TSV, and table formats.
func (m *Manager) Write(content string) error {
	// All list formats → stdout (data channel, pipeable)
	return data.Write(content)
}

// IsStructured returns true if the format is structured data (JSON, YAML, CSV, TSV).
// Note: This function is kept for backward compatibility but is no longer used
// for routing output. All list output now goes to the data channel (stdout).
func IsStructured(f format.Format) bool {
	return f == format.FormatJSON ||
		f == format.FormatYAML ||
		f == format.FormatCSV ||
		f == format.FormatTSV
}
