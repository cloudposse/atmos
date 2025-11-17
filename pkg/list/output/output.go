package output

import (
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/ui"
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
// Structured formats (JSON, YAML, CSV, TSV) go to data.Write() (stdout).
// Human-readable formats (table) go to ui.Write() (stderr).
func (m *Manager) Write(content string) error {
	if IsStructured(m.format) {
		// Structured data → stdout (pipeable)
		return data.Write(content)
	}

	// Human-readable → stderr (UI channel)
	return ui.Write(content)
}

// IsStructured returns true if the format is structured data (JSON, YAML, CSV, TSV).
func IsStructured(f format.Format) bool {
	return f == format.FormatJSON ||
		f == format.FormatYAML ||
		f == format.FormatCSV ||
		f == format.FormatTSV
}
