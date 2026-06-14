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

	facet, err := parseFacets(cmd)
	if err != nil {
		return err
	}

	v := viper.GetViper()
	if err := listParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	verbose := v.GetBool("verbose")
	outputFormat := format.Format(v.GetString(flagFormat))

	// Fully scoped (both facets) → fast single-scope path, honoring --identity.
	if facet.Stack != "" && facet.Component != "" {
		svc, err := loadServiceFn(facet)
		if err != nil {
			return err
		}
		rows := statusesToData(facet.Stack, facet.Component, svc.Status())
		empty := fmt.Sprintf("No secrets declared for component %q in stack %q.", facet.Component, facet.Stack)
		return renderSecretRows(rows, verbose, outputFormat, empty)
	}

	// Otherwise enumerate across every matching (stack, component) instance (best-effort status).
	rows, err := enumeratedSecretRows(facet)
	if err != nil {
		return err
	}
	return renderSecretRows(rows, verbose, outputFormat, emptyListMessage(facet))
}

// enumeratedSecretRows builds list rows across all (stack, component) instances matching the
// facets. Shared secrets are de-duplicated to a single row per storage location — stack-scoped
// to one row per (stack, secret) shown with a `*` component, global to one row per secret shown
// with a `*` stack and component — they are inherited into every consumer but stored once.
func enumeratedSecretRows(facet secretScope) ([]map[string]any, error) {
	entries, atmosConfig, err := enumerateScopesFn(facet)
	if err != nil {
		return nil, err
	}

	var rows []map[string]any
	seenShared := make(map[string]bool)
	for _, entry := range entries {
		svc := secrets.NewService(atmosConfig, entry.Stack, entry.Component, entry.Section)
		statuses := svc.Status()
		for i := range statuses {
			st := &statuses[i]
			switch st.Declaration.Scope {
			case secrets.ScopeGlobal:
				key := "global\x00" + st.Declaration.Name
				if seenShared[key] {
					continue
				}
				seenShared[key] = true
				rows = append(rows, statusRow("*", "*", st))
			case secrets.ScopeStack:
				key := entry.Stack + "\x00" + st.Declaration.Name
				if seenShared[key] {
					continue
				}
				seenShared[key] = true
				rows = append(rows, statusRow(entry.Stack, "*", st))
			default:
				rows = append(rows, statusRow(entry.Stack, entry.Component, st))
			}
		}
	}
	return rows, nil
}

// emptyListMessage returns the "nothing found" message scoped to the active facets.
func emptyListMessage(facet secretScope) string {
	switch {
	case facet.Stack != "":
		return fmt.Sprintf("No secrets declared in stack %q.", facet.Stack)
	case facet.Component != "":
		return fmt.Sprintf("No secrets declared for component %q in any stack.", facet.Component)
	default:
		return "No secrets declared in any stack."
	}
}

// renderSecretRows renders secret rows via the pkg/list rendering pipeline.
// It is TTY-aware: styled table on TTY, plain/delimited when piped.
func renderSecretRows(rows []map[string]any, verbose bool, outputFormat format.Format, emptyMessage string) error {
	defer perf.Track(nil, "secret.renderSecretRows")()

	if len(rows) == 0 {
		ui.Info(emptyMessage)
		return nil
	}

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
	return r.Render(rows)
}

// statusesToData converts []secrets.Status for a single (stack, component) to renderer rows.
func statusesToData(stack, component string, statuses []secrets.Status) []map[string]any {
	rows := make([]map[string]any, 0, len(statuses))
	for i := range statuses {
		rows = append(rows, statusRow(stack, component, &statuses[i]))
	}
	return rows
}

// statusRow converts a single status into a renderer row.
func statusRow(stack, component string, st *secrets.Status) map[string]any {
	return map[string]any{
		"stack":       stack,
		"component":   component,
		"secret":      st.Declaration.Name,
		"scope":       scopeLabel(st.Declaration.Scope),
		"provider":    backendLabel(&st.Declaration),
		"status":      statusLabel(st),
		"description": st.Declaration.Description,
	}
}

// scopeLabel returns the display scope for a declaration, defaulting empty to "instance".
func scopeLabel(scope secrets.Scope) string {
	switch scope {
	case secrets.ScopeStack, secrets.ScopeGlobal:
		return string(scope)
	default:
		return string(secrets.ScopeInstance)
	}
}

// secretListColumns returns column configuration for secret list output.
// When verbose is true, a Description column is appended.
func secretListColumns(verbose bool) []column.Config {
	cols := []column.Config{
		{Name: "Stack", Value: "{{ .stack }}"},
		{Name: "Component", Value: "{{ .component }}"},
		{Name: "Secret", Value: "{{ .secret }}"},
		{Name: "Scope", Value: "{{ .scope }}"},
		{Name: "Provider", Value: "{{ .provider }}"},
		{Name: "Status", Value: "{{ .status }}"},
	}
	if verbose {
		cols = append(cols, column.Config{Name: "Description", Value: "{{ .description }}"})
	}
	return cols
}

// backendLabel returns a short backend identifier for display.
func backendLabel(decl *secrets.Declaration) string {
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
