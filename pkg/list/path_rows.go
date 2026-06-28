package list

import (
	"text/template"

	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listsort "github.com/cloudposse/atmos/pkg/list/sort"
)

// PathRow is the common row shape used by config discovery commands.
type PathRow struct {
	File string
	Path string
	Type string
}

// RenderPathRows renders path discovery rows through the standard list pipeline.
func RenderPathRows(rows []PathRow, outputFormat string, delimiter string) (string, error) {
	if outputFormat == "" {
		outputFormat = string(format.FormatPaths)
	}
	if err := format.ValidateFormat(outputFormat); err != nil {
		return "", err
	}

	selector, err := column.NewSelector([]column.Config{
		{Name: "file", Value: "{{ .file }}"},
		{Name: "path", Value: "{{ .path }}"},
		{Name: "type", Value: "{{ .type }}"},
	}, template.FuncMap{})
	if err != nil {
		return "", err
	}

	data := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, map[string]any{
			"file": row.File,
			"path": row.Path,
			"type": row.Type,
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
