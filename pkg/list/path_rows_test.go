package list

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderPathRowsStructuredFormatsIncludeValue(t *testing.T) {
	rows := []PathRow{
		{File: "atmos.yaml", Path: "commands[0].steps[0]", Type: "string", Value: "echo one\necho two\n"},
	}

	tests := []struct {
		name     string
		format   string
		expected string
	}{
		{
			name:     "table",
			format:   "table",
			expected: "atmos.yaml\tcommands[0].steps[0]\tstring\techo one ... (2 lines)\n",
		},
		{
			name:     "json",
			format:   "json",
			expected: "\"value\": \"echo one ... (2 lines)\"",
		},
		{
			name:     "yaml",
			format:   "yaml",
			expected: "value: echo one ... (2 lines)",
		},
		{
			name:     "csv",
			format:   "csv",
			expected: "file,path,type,value\natmos.yaml,commands[0].steps[0],string,echo one ... (2 lines)\n",
		},
		{
			name:     "tsv",
			format:   "tsv",
			expected: "file\tpath\ttype\tvalue\natmos.yaml\tcommands[0].steps[0]\tstring\techo one ... (2 lines)\n",
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

func TestRenderPathRowsWithPattern(t *testing.T) {
	rows := []PathRow{
		{File: "atmos.yaml", Path: "commands[0].name", Type: "string", Value: "check"},
		{File: "atmos.yaml", Path: "toolchain.aliases.gofumpt", Type: "string", Value: "gofumpt"},
		{File: "atmos.yaml", Path: "toolchain.registries[0].name", Type: "string", Value: "aqua"},
	}

	output, err := RenderPathRowsWithPattern(rows, "paths", "", "toolchain.*")
	require.NoError(t, err)
	require.Equal(t, `atmos.yaml
  toolchain.aliases.gofumpt
  toolchain.registries[0].name
`, output)
}

func TestRenderPathRowsWithPatternMatchesArrayIndexes(t *testing.T) {
	rows := []PathRow{
		{File: "vendor.yaml", Path: "spec.sources[0].component", Type: "string", Value: "vpc"},
		{File: "vendor.yaml", Path: "spec.sources[0].version", Type: "string", Value: "v1.0.0"},
		{File: "vendor.yaml", Path: "spec.sources[1].version", Type: "string", Value: "v2.0.0"},
	}

	output, err := RenderPathRowsWithPattern(rows, "paths", "", "spec.sources[*].version")
	require.NoError(t, err)
	require.Equal(t, `vendor.yaml
  spec.sources[0].version
  spec.sources[1].version
`, output)
}
