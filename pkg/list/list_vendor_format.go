package list

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/list/extract"
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
func buildVendorRows(vendorInfos []extract.VendorInfo, columns []schema.ListColumnConfig) []map[string]interface{} {
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
			case ColumnNameTags:
				row[col.Name] = strings.Join(vi.Tags, ", ")
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

// mapKeys returns a copy of the map with keys mapped by the provided function.
func mapKeys(m map[string]interface{}, fn func(string) string) map[string]interface{} {
	newMap := make(map[string]interface{}, len(m))
	for k, v := range m {
		newMap[fn(k)] = v
	}
	return newMap
}

// buildVendorDataMap converts the row slice to a map keyed by component name.
func buildVendorDataMap(rows []map[string]interface{}, capitalizeKeys bool) map[string]interface{} {
	data := make(map[string]interface{})
	var keyFn func(string) string
	if capitalizeKeys {
		keyFn = func(s string) string { return s }
	} else {
		keyFn = strings.ToLower
	}
	for i, row := range rows {
		key, ok := row[ColumnNameComponent].(string)
		if !ok || key == "" {
			key = fmt.Sprintf("vendor_%d", i)
		}
		data[key] = mapKeys(row, keyFn)
	}
	return data
}

// buildVendorCSVTSV returns CSV/TSV output for vendor rows with proper header order and value rows.
func buildVendorCSVTSV(headers []string, rows []map[string]interface{}, delimiter string) string {
	var b strings.Builder
	// Write header row
	for i, h := range headers {
		b.WriteString(h)
		if i < len(headers)-1 {
			b.WriteString(delimiter)
		}
	}
	b.WriteString(NewLine)
	// Write value rows
	for _, row := range rows {
		for i, h := range headers {
			val := ""
			if v, ok := row[h]; ok && v != nil {
				val = fmt.Sprintf("%v", v)
			}
			b.WriteString(val)
			if i < len(headers)-1 {
				b.WriteString(delimiter)
			}
		}
		b.WriteString(NewLine)
	}
	return b.String()
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
