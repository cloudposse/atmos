package output

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	manager := New(format.FormatJSON)
	assert.NotNil(t, manager)
	assert.Equal(t, format.FormatJSON, manager.format)
}

func TestFormat_IsStructured(t *testing.T) {
	tests := []struct {
		format   format.Format
		expected bool
	}{
		{format.FormatJSON, true},
		{format.FormatYAML, true},
		{format.FormatCSV, true},
		{format.FormatTSV, true},
		{format.FormatTable, false},
		{format.FormatTemplate, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			result := IsStructured(tt.format)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestManager_Write(t *testing.T) {
	// Initialize I/O context and data writer for tests.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	tests := []struct {
		name    string
		format  format.Format
		content string
	}{
		{"JSON format", format.FormatJSON, `{"key":"value"}`},
		{"YAML format", format.FormatYAML, "key: value"},
		{"CSV format", format.FormatCSV, "col1,col2"},
		{"TSV format", format.FormatTSV, "col1\tcol2"},
		{"Table format", format.FormatTable, "│ Col1 │ Col2 │"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := New(tt.format)

			// Note: We can't easily test the actual output routing without
			// mocking the global io context, but we can verify the method
			// doesn't error.
			err := manager.Write(tt.content)
			assert.NoError(t, err)
		})
	}
}
