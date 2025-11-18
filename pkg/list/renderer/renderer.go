package renderer

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/output"
	"github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/terminal"
)

// Renderer orchestrates the complete list rendering pipeline.
// Pipeline: data → filter → column selection → sort → format → output.
type Renderer struct {
	filters   []filter.Filter
	selector  *column.Selector
	sorters   []*sort.Sorter
	format    format.Format
	delimiter string
	output    *output.Manager
}

// New creates a renderer with optional components.
func New(
	filters []filter.Filter,
	selector *column.Selector,
	sorters []*sort.Sorter,
	fmt format.Format,
	delimiter string,
) *Renderer {
	return &Renderer{
		filters:   filters,
		selector:  selector,
		sorters:   sorters,
		format:    fmt,
		delimiter: delimiter,
		output:    output.New(fmt),
	}
}

// Render executes the full pipeline and writes output.
func (r *Renderer) Render(data []map[string]any) error {
	// Guard against nil column selector.
	if r.selector == nil {
		return fmt.Errorf("%w: renderer created with nil column selector", errUtils.ErrInvalidConfig)
	}

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
			return fmt.Errorf("%w: filter returned invalid type: expected []map[string]any, got %T", errUtils.ErrInvalidConfig, result)
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
	formatted, err := r.formatTable(headers, rows)
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
func (r *Renderer) formatTable(headers []string, rows [][]string) (string, error) {
	f := r.format
	// Default to table format if empty.
	if f == "" {
		f = format.FormatTable
	}

	switch f {
	case format.FormatJSON:
		return formatJSON(headers, rows)
	case format.FormatYAML:
		return formatYAML(headers, rows)
	case format.FormatCSV:
		delim := r.delimiter
		if delim == "" {
			delim = ","
		}
		return formatDelimited(headers, rows, delim)
	case format.FormatTSV:
		delim := r.delimiter
		if delim == "" {
			delim = "\t"
		}
		return formatDelimited(headers, rows, delim)
	case format.FormatTable:
		return formatStyledTableOrPlain(headers, rows), nil
	default:
		return "", fmt.Errorf("%w: unsupported format: %s", errUtils.ErrInvalidConfig, f)
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

// formatStyledTableOrPlain formats output as a styled table for TTY or plain list when piped.
// When stdout is not a TTY (piped/redirected), outputs plain format without headers for backward compatibility.
// This maintains compatibility with scripts that expect simple line-by-line output.
func formatStyledTableOrPlain(headers []string, rows [][]string) string {
	// Check if stdout is a TTY
	term := terminal.New()
	isTTY := term.IsTTY(terminal.Stdout)

	if !isTTY {
		// Piped/redirected output - use plain format (no headers, no borders)
		// This matches the old behavior for backward compatibility with scripts.
		return formatPlainList(headers, rows)
	}

	// Interactive terminal - use styled table
	return format.CreateStyledTable(headers, rows)
}

// formatPlainList formats rows as a simple list (one value per line, no headers).
// Used when output is piped to maintain backward compatibility with scripts.
func formatPlainList(headers []string, rows [][]string) string {
	var lines []string

	// For single-column output, just output the values (most common case for list commands)
	if len(headers) == 1 {
		for _, row := range rows {
			if len(row) > 0 {
				lines = append(lines, row[0])
			}
		}
	} else {
		// Multi-column output when piped - use tab-separated values without headers
		// This provides structured data that can be processed by scripts
		for _, row := range rows {
			lines = append(lines, strings.Join(row, "\t"))
		}
	}

	return strings.Join(lines, "\n") + "\n"
}
