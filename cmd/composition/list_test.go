package composition

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
)

func TestCompositionListColumns(t *testing.T) {
	assert.Equal(t, []string{"Composition", "Services", "Stacks", "Description"}, columnNames(compositionListColumns(false)))
	assert.Equal(t, []string{"Composition", "Services", "Fulfilled", "Not Provided", "Unknown", "Description"}, columnNames(compositionListColumns(true)))
}

func TestCompositionListRendererSupportsStructuredFormats(t *testing.T) {
	rows := []map[string]any{{"composition": "app", "services": "api", "stacks": "local", "description": "Application"}}
	selector, err := column.NewSelector(compositionListColumns(false), column.BuildColumnFuncMap())
	require.NoError(t, err)

	for _, outputFormat := range []format.Format{format.FormatTable, format.FormatJSON, format.FormatYAML, format.FormatCSV, format.FormatTSV} {
		t.Run(string(outputFormat), func(t *testing.T) {
			output, err := renderer.New(nil, selector, nil, outputFormat, "").RenderToString(rows)
			require.NoError(t, err)
			assert.Contains(t, output, "app")
			assert.Contains(t, output, "local")
		})
	}
}

func TestCompositionListColumnsCompletion(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("stack", "", "")

	columns, _ := compositionListColumnsCompletion(cmd, nil, "")
	assert.Equal(t, []string{"Composition", "Services", "Stacks", "Description"}, columns)

	require.NoError(t, cmd.Flags().Set("stack", "local"))
	columns, _ = compositionListColumnsCompletion(cmd, nil, "")
	assert.Equal(t, []string{"Composition", "Services", "Fulfilled", "Not Provided", "Unknown", "Description"}, columns)
}

func TestValidateListFormat(t *testing.T) {
	for _, value := range []string{"", "table", "json", "yaml", "csv", "tsv"} {
		assert.NoError(t, validateListFormat(value), value)
	}
	assert.Error(t, validateListFormat("tree"))
}

func columnNames(columns []column.Config) []string {
	names := make([]string, 0, len(columns))
	for _, column := range columns {
		names = append(names, column.Name)
	}
	return names
}
