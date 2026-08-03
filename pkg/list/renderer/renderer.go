package renderer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/output"
	"github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const rendererLineEnding = "\n"

// Renderer orchestrates the complete list rendering pipeline.
// Pipeline: data → filter → column selection → sort → format → output.
type Renderer struct {
	filters      []filter.Filter
	selector     *column.Selector
	sorters      []*sort.Sorter
	format       format.Format
	delimiter    string
	output       *output.Manager
	tableOptions *format.TableOptions
}

// Option customizes optional renderer behavior.
type Option func(*Renderer)

// WithTableOptions overrides styled-table behavior (e.g. disabling semantic
// cell coloring for tables whose values are plain data rather than statuses).
func WithTableOptions(tableOptions format.TableOptions) Option {
	return func(r *Renderer) {
		r.tableOptions = &tableOptions
	}
}

// New creates a renderer with optional components.
func New(
	filters []filter.Filter,
	selector *column.Selector,
	sorters []*sort.Sorter,
	fmt format.Format,
	delimiter string,
	opts ...Option,
) *Renderer {
	r := &Renderer{
		filters:   filters,
		selector:  selector,
		sorters:   sorters,
		format:    fmt,
		delimiter: delimiter,
		output:    output.New(fmt),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// RenderToString executes the full pipeline and returns formatted output.
// This is useful when the caller needs to process the output (e.g., pass to pager).
func (r *Renderer) RenderToString(data []map[string]any) (string, error) {
	// Guard against nil column selector.
	if r.selector == nil {
		return "", fmt.Errorf("%w: renderer created with nil column selector", errUtils.ErrInvalidConfig)
	}

	// Step 1: Apply filters (AND logic).
	filtered := data
	if len(r.filters) > 0 {
		chain := filter.NewChain(r.filters...)
		result, err := chain.Apply(filtered)
		if err != nil {
			return "", fmt.Errorf("filtering failed: %w", err)
		}
		var ok bool
		filtered, ok = result.([]map[string]any)
		if !ok {
			return "", fmt.Errorf("%w: filter returned invalid type: expected []map[string]any, got %T", errUtils.ErrInvalidConfig, result)
		}
	}

	// Step 2: Extract columns with template evaluation.
	headers, rows, err := r.selector.Extract(filtered)
	if err != nil {
		return "", fmt.Errorf("column extraction failed: %w", err)
	}

	// Step 3: Sort rows.
	if len(r.sorters) > 0 {
		ms := sort.NewMultiSorter(r.sorters...)
		if err := ms.Sort(rows, headers); err != nil {
			return "", fmt.Errorf("sorting failed: %w", err)
		}
	}

	// Step 4: Format output.
	formatted, err := r.formatTable(headers, rows)
	if err != nil {
		return "", fmt.Errorf("formatting failed: %w", err)
	}

	return formatted, nil
}

// Render executes the full pipeline and writes output.
func (r *Renderer) Render(data []map[string]any) error {
	formatted, err := r.RenderToString(data)
	if err != nil {
		return err
	}

	// Write to appropriate stream.
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
	case format.FormatPaths:
		return formatPaths(headers, rows), nil
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
		return r.formatStyledTableOrPlain(headers, rows), nil
	default:
		return "", fmt.Errorf("%w: unsupported format: %s", errUtils.ErrInvalidConfig, f)
	}
}

// formatPaths groups rows by their file column and prints indented path values.
func formatPaths(headers []string, rows [][]string) string {
	term := terminal.New()
	if term.IsTTY(terminal.Stdout) {
		return formatStyledPaths(headers, rows)
	}
	return formatPlainPaths(headers, rows)
}

func formatPlainPaths(headers []string, rows [][]string) string {
	fileIndex := columnIndex(headers, "file")
	pathIndex := columnIndex(headers, "path")
	if fileIndex < 0 || pathIndex < 0 {
		return ""
	}

	var lines []string
	currentFile := ""
	for _, row := range rows {
		if fileIndex >= len(row) || pathIndex >= len(row) {
			continue
		}
		file := row[fileIndex]
		if file != currentFile {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, file)
			currentFile = file
		}
		lines = append(lines, "  "+row[pathIndex])
	}

	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, rendererLineEnding) + rendererLineEnding
}

func formatStyledPaths(headers []string, rows [][]string) string {
	indexes := pathColumnIndexes{
		file:  columnIndex(headers, "file"),
		path:  columnIndex(headers, "path"),
		typ:   columnIndex(headers, "type"),
		value: columnIndex(headers, "value"),
	}
	if !indexes.valid() {
		return ""
	}

	styles := styledPathStyles()
	widths := styledPathWidths(rows, indexes)
	var lines []string
	currentFile := ""
	opts := styledPathLineOptions{
		indexes: indexes,
		widths:  widths,
		styles:  &styles,
	}
	for _, row := range rows {
		nextLines, nextFile, ok := appendStyledPathLine(lines, currentFile, row, opts)
		if !ok {
			continue
		}
		lines = nextLines
		currentFile = nextFile
	}

	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, rendererLineEnding) + rendererLineEnding
}

type pathColumnIndexes struct {
	file  int
	path  int
	typ   int
	value int
}

func (i pathColumnIndexes) valid() bool {
	return i.file >= 0 && i.path >= 0
}

type pathStyles struct {
	file lipgloss.Style
	path lipgloss.Style
	meta lipgloss.Style
}

type pathColumnWidths struct {
	path int
	typ  int
}

type styledPathLineOptions struct {
	indexes pathColumnIndexes
	widths  pathColumnWidths
	styles  *pathStyles
}

func styledPathStyles() pathStyles {
	styles := theme.GetCurrentStyles()
	fileStyle := lipgloss.NewStyle().Bold(true)
	pathStyle := lipgloss.NewStyle()
	metaStyle := lipgloss.NewStyle().Faint(true)
	if styles != nil {
		fileStyle = fileStyle.Inherit(styles.TableHeader)
		pathStyle = pathStyle.Inherit(styles.TableRow)
		metaStyle = metaStyle.Inherit(styles.Muted)
	}
	return pathStyles{file: fileStyle, path: pathStyle, meta: metaStyle}
}

func styledPathWidths(rows [][]string, indexes pathColumnIndexes) pathColumnWidths {
	widths := pathColumnWidths{}
	for _, row := range rows {
		if indexes.path < len(row) {
			widths.path = max(widths.path, lipgloss.Width(row[indexes.path]))
		}
		if indexes.typ >= 0 && indexes.typ < len(row) {
			widths.typ = max(widths.typ, lipgloss.Width(row[indexes.typ]))
		}
	}
	return widths
}

func appendStyledPathLine(lines []string, currentFile string, row []string, opts styledPathLineOptions) ([]string, string, bool) {
	indexes := opts.indexes
	widths := opts.widths
	styles := opts.styles
	if indexes.file >= len(row) || indexes.path >= len(row) {
		return lines, currentFile, false
	}

	file := row[indexes.file]
	if file != currentFile {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, styles.file.Render(file))
		currentFile = file
	}

	line := "  " + styles.path.Render(padCell(row[indexes.path], widths.path))
	if indexes.typ >= 0 && indexes.typ < len(row) {
		line += "  " + styles.meta.Render(padCell(row[indexes.typ], widths.typ))
	}
	if indexes.value >= 0 && indexes.value < len(row) && row[indexes.value] != "" {
		line += "  " + styles.meta.Render(previewRendererCell(row[indexes.value]))
	}
	return append(lines, line), currentFile, true
}

func padCell(value string, width int) string {
	padding := width - lipgloss.Width(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

func previewRendererCell(value string) string {
	normalized := strings.ReplaceAll(value, "\r\n", rendererLineEnding)
	normalized = strings.ReplaceAll(normalized, "\r", rendererLineEnding)
	if !strings.Contains(normalized, rendererLineEnding) {
		return normalized
	}
	lines := strings.Split(strings.TrimSuffix(normalized, rendererLineEnding), rendererLineEnding)
	if len(lines) <= 1 {
		return lines[0]
	}
	return fmt.Sprintf("%s ... (%d lines)", lines[0], len(lines))
}

func columnIndex(headers []string, name string) int {
	for i, header := range headers {
		if strings.EqualFold(header, name) {
			return i
		}
	}
	return -1
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
	return string(jsonBytes) + "\n", nil
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
	return strings.Join(lines, "\n") + "\n", nil
}

// formatStyledTableOrPlain formats output as a styled table for TTY or plain list when piped.
// When stdout is not a TTY (piped/redirected), outputs plain format without headers for backward compatibility.
// This maintains compatibility with scripts that expect simple line-by-line output.
func (r *Renderer) formatStyledTableOrPlain(headers []string, rows [][]string) string {
	// Check if stdout is a TTY
	term := terminal.New()
	isTTY := term.IsTTY(terminal.Stdout)

	if !isTTY {
		// Piped/redirected output - use plain format (no headers, no borders)
		// This matches the old behavior for backward compatibility with scripts.
		return formatPlainList(headers, rows)
	}

	// Interactive terminal - use styled table
	if r.tableOptions != nil {
		return format.CreateStyledTableWithOptions(headers, rows, *r.tableOptions)
	}
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

	return strings.Join(lines, rendererLineEnding) + rendererLineEnding
}
