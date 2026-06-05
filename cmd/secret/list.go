package secret

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/secrets"
)

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
	)
	listParser.RegisterFlags(listCmd)
}

func runSecretList(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "secret.runSecretList")()

	scope, err := parseScope(cmd)
	if err != nil {
		return err
	}

	svc, err := loadService(scope)
	if err != nil {
		return err
	}

	statuses := svc.Status()
	return renderSecretStatuses(scope, statuses)
}

// renderSecretStatuses prints a simple, pipeline-friendly table of secret statuses.
func renderSecretStatuses(scope secretScope, statuses []secrets.Status) error {
	if len(statuses) == 0 {
		return data.Writeln(fmt.Sprintf("No secrets declared for component %q in stack %q.", scope.Component, scope.Stack))
	}

	rows := [][]string{{"STACK", "COMPONENT", "SECRET", "PROVIDER", "STATUS"}}
	for i := range statuses {
		st := &statuses[i]
		rows = append(rows, []string{
			scope.Stack,
			scope.Component,
			st.Declaration.Name,
			backendLabel(st.Declaration),
			statusLabel(st),
		})
	}
	return data.Writeln(formatTable(rows))
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

// formatTable renders rows as a left-aligned, space-padded table.
func formatTable(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	var b strings.Builder
	for _, row := range rows {
		for i, cell := range row {
			b.WriteString(cell)
			if i < len(row)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-len(cell)+2))
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
