package list

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderPathRowsStructuredFormatsIncludeValue(t *testing.T) {
	rows := []PathRow{
		{File: "atmos.yaml", Path: "logs.level", Type: "string", Value: "info"},
	}

	tests := []struct {
		name     string
		format   string
		expected string
	}{
		{
			name:     "table",
			format:   "table",
			expected: "atmos.yaml\tlogs.level\tstring\tinfo\n",
		},
		{
			name:     "json",
			format:   "json",
			expected: "\"value\": \"info\"",
		},
		{
			name:     "yaml",
			format:   "yaml",
			expected: "value: info",
		},
		{
			name:     "csv",
			format:   "csv",
			expected: "file,path,type,value\natmos.yaml,logs.level,string,info\n",
		},
		{
			name:     "tsv",
			format:   "tsv",
			expected: "file\tpath\ttype\tvalue\natmos.yaml\tlogs.level\tstring\tinfo\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := RenderPathRows(rows, tt.format, "")
			require.NoError(t, err)
			if tt.format == "json" || tt.format == "yaml" {
				require.Contains(t, output, tt.expected)
				return
			}
			require.Equal(t, tt.expected, output)
		})
	}
}
