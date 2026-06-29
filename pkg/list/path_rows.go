package list

import (
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listsort "github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/perf"
)

const pathRowLineEnding = "\n"

// PathRow is the common row shape used by config discovery commands.
type PathRow struct {
	File  string
	Path  string
	Type  string
	Value string
}

// RenderPathRows renders path discovery rows through the standard list pipeline.
func RenderPathRows(rows []PathRow, outputFormat string, delimiter string) (string, error) {
	return RenderPathRowsWithPattern(rows, outputFormat, delimiter, "")
}

// RenderPathRowsWithPattern renders path discovery rows after applying an optional path glob.
func RenderPathRowsWithPattern(rows []PathRow, outputFormat string, delimiter string, pathPattern string) (string, error) {
	defer perf.Track(nil, "list.RenderPathRowsWithPattern")()

	if outputFormat == "" {
		outputFormat = string(format.FormatPaths)
	}
	if err := format.ValidateFormat(outputFormat); err != nil {
		return "", err
	}
	filteredRows, err := filterPathRows(rows, pathPattern)
	if err != nil {
		return "", err
	}

	selector, err := column.NewSelector([]column.Config{
		{Name: "file", Value: "{{ .file }}"},
		{Name: "path", Value: "{{ .path }}"},
		{Name: "type", Value: "{{ .type }}"},
		{Name: "value", Value: "{{ .value }}"},
	}, template.FuncMap{})
	if err != nil {
		return "", err
	}

	data := make([]map[string]any, 0, len(filteredRows))
	for _, row := range filteredRows {
		data = append(data, map[string]any{
			"file":  row.File,
			"path":  row.Path,
			"type":  row.Type,
			"value": previewPathRowValue(row.Value),
		})
	}

	r := renderer.New(
		nil,
		selector,
		[]*listsort.Sorter{
			listsort.NewSorter("file", listsort.Ascending),
			listsort.NewSorter("path", listsort.Ascending),
		},
		format.Format(outputFormat),
		delimiter,
	)
	return r.RenderToString(data)
}

func filterPathRows(rows []PathRow, pathPattern string) ([]PathRow, error) {
	if pathPattern == "" {
		return rows, nil
	}

	pattern := pathPatternRegexp(pathPattern)
	filtered := make([]PathRow, 0, len(rows))
	for _, row := range rows {
		if pattern.MatchString(row.Path) {
			filtered = append(filtered, row)
		}
	}
	return filtered, nil
}

func pathPatternRegexp(pathPattern string) *regexp.Regexp {
	quoted := regexp.QuoteMeta(pathPattern)
	quoted = strings.ReplaceAll(quoted, `\*`, `.*`)
	quoted = strings.ReplaceAll(quoted, `\?`, `.`)
	return regexp.MustCompile("^" + quoted + "$")
}

func previewPathRowValue(value string) string {
	normalized := strings.ReplaceAll(value, "\r\n", pathRowLineEnding)
	normalized = strings.ReplaceAll(normalized, "\r", pathRowLineEnding)
	if !strings.Contains(normalized, pathRowLineEnding) {
		return normalized
	}
	lines := strings.Split(strings.TrimSuffix(normalized, pathRowLineEnding), pathRowLineEnding)
	if len(lines) <= 1 {
		return lines[0]
	}
	return fmt.Sprintf("%s ... (%d lines)", lines[0], len(lines))
}
