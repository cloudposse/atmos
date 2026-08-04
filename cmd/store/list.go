package store

import (
	_ "embed"
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

//go:embed markdown/atmos_store_list_usage.md
var storeListUsageMarkdown string

// flagFormat is the name of the output-format flag.
const flagFormat = "format"

var listParser *flags.StandardParser

var listCmd = &cobra.Command{
	Use:   "list [STORE]",
	Short: "List configured store backends, or the key/value pairs stored inside one.",
	Long: "List the store backends configured under `stores:` in atmos.yaml, showing each one's kind and " +
		"capabilities. Pass a STORE name to list the key/value pairs stored inside that backend instead, " +
		"scoped with --stack and --component the same way `atmos store set` writes a key. Only backends " +
		"that support key enumeration can list their contents this way; check the Listable column first.",
	Example: storeListUsageMarkdown,
	Args:    cobra.MaximumNArgs(1),
	RunE:    runStoreList,
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

	if len(args) == 1 {
		return runStoreListKeyValues(args[0], scope, outputFormat)
	}

	svc, err := loadServiceForListFn(scope)
	if err != nil {
		return err
	}

	rows := descriptorsToRows(svc.List())
	return renderRows(rows, storeColumns, "Name", "No stores configured.", outputFormat)
}

// runStoreListKeyValues lists the key/value pairs stored inside a single store -- the
// `atmos store list STORE` form. Unlike the bare `store list`, this needs real backend access, so
// it loads the service through the authenticated loader.
func runStoreListKeyValues(storeName string, scope storeScope, outputFormat format.Format) error {
	svc, err := loadServiceFn(scope)
	if err != nil {
		return err
	}

	kvs, err := svc.ListKeyValues(storeName, scope.Stack, scope.Component)
	if err != nil {
		return err
	}

	// Register every value with the masker before rendering, the same way `atmos store get`
	// does, but only for a `secret: true` store -- a non-secret store's values are shown as-is.
	secret := svc.IsSecret(storeName)

	rows := make([]map[string]any, 0, len(kvs))
	for _, kv := range kvs {
		if secret {
			registerSecretValueFn(kv.Value)
		}
		rows = append(rows, map[string]any{
			"stack":     scope.Stack,
			"component": scope.Component,
			"key":       kv.Key,
			"value":     kv.Value,
		})
	}

	empty := fmt.Sprintf("No keys found in store %q.", storeName)
	return renderRows(rows, keyValueColumns, "Key", empty, outputFormat)
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

// storeColumns is the column configuration for the bare `atmos store list` (configured backends).
var storeColumns = []column.Config{
	{Name: "Name", Value: "{{ .name }}"},
	{Name: "Kind", Value: "{{ .kind }}"},
	{Name: "Secret", Value: "{{ .secret }}"},
	{Name: "Deletable", Value: "{{ .deletable }}"},
	{Name: "Has Status", Value: "{{ .hasStatus }}"},
	{Name: "Local", Value: "{{ .local }}"},
	{Name: "Listable", Value: "{{ .listable }}"},
}

// keyValueColumns is the column configuration for `atmos store list STORE` (key/value contents).
var keyValueColumns = []column.Config{
	{Name: "Stack", Value: "{{ .stack }}"},
	{Name: "Component", Value: "{{ .component }}"},
	{Name: "Key", Value: "{{ .key }}"},
	{Name: "Value", Value: "{{ .value }}"},
}

// renderRows renders rows via the pkg/list rendering pipeline, sorted by sortField. It is
// TTY-aware: styled table on TTY, plain/delimited when piped.
func renderRows(rows []map[string]any, columns []column.Config, sortField, emptyMessage string, outputFormat format.Format) error {
	defer perf.Track(nil, "store.renderRows")()

	if len(rows) == 0 {
		ui.Info(emptyMessage)
		return nil
	}

	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("error creating column selector: %w", err)
	}

	sorters := []*listSort.Sorter{listSort.NewSorter(sortField, listSort.Ascending)}
	var filters []filter.Filter

	r := renderer.New(filters, selector, sorters, outputFormat, "")
	return r.Render(rows)
}
