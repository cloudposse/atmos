package list

import (
	"errors"
	"fmt"

	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// NewLine is the newline character.
	NewLine = "\n"
	// DefaultTerminalWidth is the default terminal width.
	DefaultTerminalWidth = 80
	// MaxColumnWidth is the maximum width for a column.
	MaxColumnWidth = 30
)

var (
	// ErrUnsupportedFormat is returned when an unsupported format is specified.
	ErrUnsupportedFormat = errors.New("unsupported format")
	// ErrInvalidVendorData is returned when vendor data is invalid.
	ErrInvalidVendorData = errors.New("invalid vendor data")
)

// buildVendorRows constructs the slice of row maps for the vendor table.
func buildVendorRows(vendorInfos []VendorInfo, columns []schema.ListColumnConfig) []map[string]interface{} {
	var rows []map[string]interface{}
	for _, vi := range vendorInfos {
		row := make(map[string]interface{})
		for _, col := range columns {
			switch col.Name {
			case ColumnNameComponent:
				row[col.Name] = vi.Component
			case ColumnNameType:
				row[col.Name] = vi.Type
			case ColumnNameManifest:
				row[col.Name] = vi.Manifest
			case ColumnNameFolder:
				row[col.Name] = vi.Folder
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// prepareVendorHeaders builds the custom header slice for the vendor table.
func prepareVendorHeaders(columns []schema.ListColumnConfig) []string {
	var headers []string
	for _, col := range columns {
		headers = append(headers, col.Name)
	}
	return headers
}

// buildVendorDataMap converts the row slice to a map keyed by component name.
func buildVendorDataMap(rows []map[string]interface{}) map[string]interface{} {
	data := make(map[string]interface{})
	for i, row := range rows {
		key, ok := row[ColumnNameComponent].(string)
		if !ok || key == "" {
			key = fmt.Sprintf("vendor_%d", i)
		}
		data[key] = row
	}
	return data
}

// renderVendorTableOutput formats a row-oriented vendor table for TTY.
func renderVendorTableOutput(headers []string, rows []map[string]interface{}) string {
	var tableRows [][]string
	for _, row := range rows {
		var rowVals []string
		for _, col := range headers {
			val := ""
			if v, ok := row[col]; ok && v != nil {
				val = fmt.Sprintf("%v", v)
			}
			rowVals = append(rowVals, val)
		}
		tableRows = append(tableRows, rowVals)
	}
	return format.CreateStyledTable(headers, tableRows)
}
