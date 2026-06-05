package secret

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
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/ui"
)

// flagFormat is the name of the output-format flag.
const flagFormat = "format"

var listParser *flags.StandardParser

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List declared secrets and their initialization status.",
	Long:  "List the secrets declared for a component in a stack, showing each secret's backend provider and whether it is initialized.",
	Args:  cobra.NoArgs,
	RunE:  runSecretList,
}

func init() {
	listParser = flags.NewStandardParser(
		flags.WithBoolFlag("verbose", "v", false, "Show declaration descriptions"),
		flags.WithStringFlag(flagFormat, "f", "", "Output format: table, json, yaml, csv, tsv"),
		flags.WithEnvVars(flagFormat, "ATMOS_SECRET_LIST_FORMAT"),
		flags.WithValidValues(flagFormat, "table", "json", "yaml", "csv", "tsv"),
	)
	listParser.RegisterFlags(listCmd)

	if err := listParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func runSecretList(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretList")()

	scope, err := parseScope(cmd)
	if err != nil {
		return err
	}

	v := viper.GetViper()
	if err := listParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	verbose := v.GetBool("verbose")
	outputFormat := format.Format(v.GetString(flagFormat))

	svc, err := loadService(scope)
	if err != nil {
		return err
	}

	statuses := svc.Status()
	return renderSecretStatuses(scope, statuses, verbose, outputFormat)
}

// renderSecretStatuses renders secret statuses via the pkg/list rendering pipeline.
// It is TTY-aware: styled table on TTY, plain/delimited when piped.
func renderSecretStatuses(scope secretScope, statuses []secrets.Status, verbose bool, outputFormat format.Format) error {
	defer perf.Track(nil, "secret.renderSecretStatuses")()

	if len(statuses) == 0 {
		ui.Info(fmt.Sprintf("No secrets declared for component %q in stack %q.", scope.Component, scope.Stack))
		return nil
	}

	// Convert statuses to []map[string]any for the rendering pipeline.
	data := statusesToData(scope, statuses)

	// Build column configuration (with optional Description when --verbose).
	columns := secretListColumns(verbose)

	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("error creating column selector: %w", err)
	}

	// Default sort: stack ascending, secret ascending.
	sorters := []*listSort.Sorter{
		listSort.NewSorter("Stack", listSort.Ascending),
		listSort.NewSorter("Secret", listSort.Ascending),
	}

	var filters []filter.Filter

	r := renderer.New(filters, selector, sorters, outputFormat, "")
	return r.Render(data)
}

// statusesToData converts []secrets.Status to the generic map slice expected by the renderer.
func statusesToData(scope secretScope, statuses []secrets.Status) []map[string]any {
	rows := make([]map[string]any, 0, len(statuses))
	for i := range statuses {
		st := &statuses[i]
		rows = append(rows, map[string]any{
			"stack":       scope.Stack,
			"component":   scope.Component,
			"secret":      st.Declaration.Name,
			"provider":    backendLabel(st.Declaration),
			"status":      statusLabel(st),
			"description": st.Declaration.Description,
		})
	}
	return rows
}

// secretListColumns returns column configuration for secret list output.
// When verbose is true, a Description column is appended.
func secretListColumns(verbose bool) []column.Config {
	cols := []column.Config{
		{Name: "Stack", Value: "{{ .stack }}"},
		{Name: "Component", Value: "{{ .component }}"},
		{Name: "Secret", Value: "{{ .secret }}"},
		{Name: "Provider", Value: "{{ .provider }}"},
		{Name: "Status", Value: "{{ .status }}"},
	}
	if verbose {
		cols = append(cols, column.Config{Name: "Description", Value: "{{ .description }}"})
	}
	return cols
}

// backendLabel returns a short backend identifier for display.
func backendLabel(decl secrets.Declaration) string {
	if decl.BackendName == "" {
		return "(none)"
	}
	return string(decl.BackendType) + ":" + decl.BackendName
}

// statusLabel returns the initialization status text for a secret.
func statusLabel(st *secrets.Status) string {
	if st.Err != nil {
		return "error"
	}
	if st.Initialized {
		return "initialized"
	}
	return "missing"
}
