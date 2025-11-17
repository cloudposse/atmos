package renderer

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/output"
	"github.com/cloudposse/atmos/pkg/list/sort"
)

// Renderer orchestrates the complete list rendering pipeline.
// Pipeline: data → filter → column selection → sort → format → output.
type Renderer struct {
	filters  []filter.Filter
	selector *column.Selector
	sorters  []*sort.Sorter
	format   format.Format
	output   *output.Manager
}

// New creates a renderer with optional components.
func New(
	filters []filter.Filter,
	selector *column.Selector,
	sorters []*sort.Sorter,
	fmt format.Format,
) *Renderer {
	return &Renderer{
		filters:  filters,
		selector: selector,
		sorters:  sorters,
		format:   fmt,
		output:   output.New(fmt),
	}
}

// Render executes the full pipeline and writes output.
func (r *Renderer) Render(data []map[string]any) error {
	// Step 1: Apply filters (AND logic).
	filtered := data
	if len(r.filters) > 0 {
		chain := filter.NewChain(r.filters...)
		result, err := chain.Apply(filtered)
		if err != nil {
			return fmt.Errorf("filtering failed: %w", err)
		}
		var ok bool
		filtered, ok = result.([]map[string]any)
		if !ok {
			return fmt.Errorf("filter returned invalid type: expected []map[string]any, got %T", result)
		}
	}

	// Step 2: Extract columns with template evaluation.
	headers, rows, err := r.selector.Extract(filtered)
	if err != nil {
		return fmt.Errorf("column extraction failed: %w", err)
	}

	// Step 3: Sort rows.
	if len(r.sorters) > 0 {
		ms := sort.NewMultiSorter(r.sorters...)
		if err := ms.Sort(rows, headers); err != nil {
			return fmt.Errorf("sorting failed: %w", err)
		}
	}

	// Step 4: Format output.
	formatted, err := formatTable(headers, rows, r.format)
	if err != nil {
		return fmt.Errorf("formatting failed: %w", err)
	}

	// Step 5: Write to appropriate stream.
	if err := r.output.Write(formatted); err != nil {
		return fmt.Errorf("output failed: %w", err)
	}

	return nil
}

// formatTable formats headers and rows into the requested format.
func formatTable(headers []string, rows [][]string, f format.Format) (string, error) {
	switch f {
	case format.FormatJSON:
		return formatJSON(headers, rows)
	case format.FormatYAML:
		return formatYAML(headers, rows)
	case format.FormatCSV:
		return formatDelimited(headers, rows, ",")
	case format.FormatTSV:
		return formatDelimited(headers, rows, "\t")
	case format.FormatTable:
		return formatStyledTable(headers, rows), nil
	default:
		return "", fmt.Errorf("unsupported format: %s", f)
	}
}

// formatJSON formats headers and rows as JSON array of objects.
func formatJSON(headers []string, rows [][]string) (string, error) {
	var result []map[string]string
	for _, row := range rows {
		obj := make(map[string]string)
		for i, header := range headers {
			if i < len(row) {
				obj[header] = row[i]
			}
		}
		result = append(result, obj)
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// formatYAML formats headers and rows as YAML array of objects.
func formatYAML(headers []string, rows [][]string) (string, error) {
	var result []map[string]string
	for _, row := range rows {
		obj := make(map[string]string)
		for i, header := range headers {
			if i < len(row) {
				obj[header] = row[i]
			}
		}
		result = append(result, obj)
	}

	yamlBytes, err := yaml.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}

// formatDelimited formats headers and rows as CSV or TSV.
func formatDelimited(headers []string, rows [][]string, delimiter string) (string, error) {
	var lines []string
	lines = append(lines, strings.Join(headers, delimiter))
	for _, row := range rows {
		lines = append(lines, strings.Join(row, delimiter))
	}
	return strings.Join(lines, "\n"), nil
}

// formatStyledTable formats headers and rows as a styled table using existing helper.
func formatStyledTable(headers []string, rows [][]string) string {
	return format.CreateStyledTable(headers, rows)
}
