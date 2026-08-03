package store

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/perf"
	pstore "github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/ui"
)

// flagFormat is the name of the output-format flag.
const flagFormat = "format"

var listParser *flags.StandardParser

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured store backends.",
	Long:  "List the store backends configured under `stores:` in atmos.yaml, showing each one's kind and capabilities.",
	Args:  cobra.NoArgs,
	RunE:  runStoreList,
}

func init() {
	listParser = flags.NewStandardParser(
		flags.WithStringFlag(flagFormat, "f", "", "Output format: table, json, yaml, csv, tsv"),
		flags.WithEnvVars(flagFormat, "ATMOS_STORE_LIST_FORMAT"),
		flags.WithValidValues(flagFormat, "table", "json", "yaml", "csv", "tsv"),
	)
	listParser.RegisterFlags(listCmd)

	if err := listParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func runStoreList(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "store.runStoreList")()

	scope, err := parseStoreScope(cmd)
	if err != nil {
		return err
	}

	v := viper.GetViper()
	if err := listParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}
	outputFormat := format.Format(v.GetString(flagFormat))

	svc, err := loadServiceForListFn(scope)
	if err != nil {
		return err
	}

	rows := descriptorsToRows(svc.List())
	return renderStoreRows(rows, outputFormat)
}

// descriptorsToRows converts []pstore.Descriptor to renderer rows.
func descriptorsToRows(descriptors []pstore.Descriptor) []map[string]any {
	rows := make([]map[string]any, 0, len(descriptors))
	for i := range descriptors {
		d := &descriptors[i]
		rows = append(rows, map[string]any{
			"name":      d.Name,
			"kind":      d.Kind,
			"secret":    d.Secret,
			"deletable": d.Deletable,
			"hasStatus": d.HasStatus,
			"local":     d.Local,
			"listable":  d.Listable,
		})
	}
	return rows
}

// renderStoreRows renders store rows via the pkg/list rendering pipeline. It is TTY-aware:
// styled table on TTY, plain/delimited when piped.
func renderStoreRows(rows []map[string]any, outputFormat format.Format) error {
	defer perf.Track(nil, "store.renderStoreRows")()

	if len(rows) == 0 {
		ui.Info("No stores configured.")
		return nil
	}

	columns := []column.Config{
		{Name: "Name", Value: "{{ .name }}"},
		{Name: "Kind", Value: "{{ .kind }}"},
		{Name: "Secret", Value: "{{ .secret }}"},
		{Name: "Deletable", Value: "{{ .deletable }}"},
		{Name: "Local", Value: "{{ .local }}"},
		{Name: "Listable", Value: "{{ .listable }}"},
	}

	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("error creating column selector: %w", err)
	}

	sorters := []*listSort.Sorter{listSort.NewSorter("Name", listSort.Ascending)}
	var filters []filter.Filter

	r := renderer.New(filters, selector, sorters, outputFormat, "")
	return r.Render(rows)
}
