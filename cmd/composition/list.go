package composition

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	pkgcomposition "github.com/cloudposse/atmos/pkg/composition"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	listFlagColumns   = "columns"
	listFlagFormat    = "format"
	listFlagSort      = "sort"
	listFlagDelimiter = "delimiter"
)

var compositionListParser *flags.StandardParser

type listOptions struct {
	Columns   []string
	Format    string
	Sort      string
	Delimiter string
}

func init() {
	compositionListParser = flags.NewStandardParser(
		flags.WithStringSliceFlag(listFlagColumns, "", nil, "Columns to display (comma-separated)"),
		flags.WithStringFlag(listFlagFormat, "f", "", "Output format: table, json, yaml, csv, tsv"),
		flags.WithStringFlag(listFlagSort, "", "", "Sort columns (for example, composition:asc)"),
		flags.WithStringFlag(listFlagDelimiter, "", "", "Delimiter for CSV/TSV output"),
		flags.WithEnvVars(listFlagColumns, "ATMOS_COMPOSITION_LIST_COLUMNS"),
		flags.WithEnvVars(listFlagFormat, "ATMOS_COMPOSITION_LIST_FORMAT"),
		flags.WithEnvVars(listFlagSort, "ATMOS_COMPOSITION_LIST_SORT"),
		flags.WithEnvVars(listFlagDelimiter, "ATMOS_COMPOSITION_LIST_DELIMITER"),
	)
	compositionListParser.RegisterFlags(listCmd)
	if err := compositionListParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
	if err := listCmd.RegisterFlagCompletionFunc(listFlagColumns, compositionListColumnsCompletion); err != nil {
		panic(fmt.Sprintf("composition list: register columns completion: %v", err))
	}
}

func runList(cmd *cobra.Command) error {
	v := viper.GetViper()
	if err := compositionParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}
	if err := compositionListParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	options := listOptions{
		Columns:   v.GetStringSlice(listFlagColumns),
		Format:    v.GetString(listFlagFormat),
		Sort:      v.GetString(listFlagSort),
		Delimiter: v.GetString(listFlagDelimiter),
	}
	if err := validateListFormat(options.Format); err != nil {
		return err
	}

	info := buildConfigAndStacksInfo(cmd)
	rows, err := pkgcomposition.ListRows(cmd.Context(), &info)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		ui.Info("No compositions declared")
		return nil
	}

	columns := compositionListColumns(info.Stack != "")
	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return err
	}
	if err := selector.Select(options.Columns); err != nil {
		return err
	}
	sorters, err := compositionListSorters(options.Sort)
	if err != nil {
		return err
	}
	return renderer.New(nil, selector, sorters, format.Format(options.Format), options.Delimiter).Render(rows)
}

func compositionListColumns(stackScoped bool) []column.Config {
	columns := []column.Config{
		{Name: "Composition", Value: "{{ .composition }}"},
		{Name: "Services", Value: "{{ .services }}"},
	}
	if stackScoped {
		return append(
			columns,
			column.Config{Name: "Fulfilled", Value: "{{ .fulfilled }}"},
			column.Config{Name: "Not Provided", Value: "{{ .not_provided }}"},
			column.Config{Name: "Unknown", Value: "{{ .unknown }}"},
			column.Config{Name: "Description", Value: "{{ .description }}"},
		)
	}
	return append(
		columns,
		column.Config{Name: "Stacks", Value: "{{ .stacks }}"},
		column.Config{Name: "Description", Value: "{{ .description }}"},
	)
}

func compositionListColumnsCompletion(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	stackScoped := false
	if stackFlag := cmd.Flag(stackFlagName); stackFlag != nil {
		stackScoped = stackFlag.Value.String() != ""
	}
	columns := compositionListColumns(stackScoped)
	names := make([]string, 0, len(columns))
	for _, column := range columns {
		names = append(names, column.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func compositionListSorters(spec string) ([]*listSort.Sorter, error) {
	if spec != "" {
		return listSort.ParseSortSpec(spec)
	}
	return []*listSort.Sorter{listSort.NewSorter("Composition", listSort.Ascending)}, nil
}

func validateListFormat(value string) error {
	switch format.Format(value) {
	case "", format.FormatTable, format.FormatJSON, format.FormatYAML, format.FormatCSV, format.FormatTSV:
		return nil
	default:
		return fmt.Errorf("%w: unsupported composition list format %q (supported: table, json, yaml, csv, tsv)", errUtils.ErrInvalidFlag, value)
	}
}
