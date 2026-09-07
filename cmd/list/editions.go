package list

import (
	"fmt"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

var editionsParser *flags.StandardParser

// EditionsOptions contains parsed flags for the editions command.
type EditionsOptions struct {
	global.Flags
	Format string
	From   string
	To     string
}

// editionsCmd lists the journal of default changes between editions.
var editionsCmd = &cobra.Command{
	Use:   "editions",
	Short: "List the journal of default changes between Atmos editions",
	Long: `Display the editions journal: every change to a previously shipped default,
with the date it changed, the old and new values, and the pull request that changed it.
Pin a project to an edition with the top-level "edition" setting in atmos.yaml
(or ATMOS_EDITION / --edition) to keep the defaults from that date.

Use --from and --to to diff two editions: only the changes that took effect
between the two anchors are shown. Anchors accept YYYY, YYYY-MM, or YYYY-MM-DD.`,
	Example: "atmos list editions\n" +
		"atmos list editions --format=json\n" +
		"atmos list editions --from=2025 --to=2026\n" +
		"atmos list editions --from=2025-10",
	Args: cobra.NoArgs,
	RunE: executeListEditions,
}

func init() {
	editionsParser = NewListParser(
		WithFormatFlag,
		withEditionRangeFlags,
	)

	editionsParser.RegisterFlags(editionsCmd)

	if err := editionsParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

// withEditionRangeFlags adds the --from/--to anchor flags for diffing editions.
func withEditionRangeFlags(options *[]flags.Option) {
	*options = append(
		*options,
		flags.WithStringFlag("from", "", "", "Show only changes after this edition anchor (YYYY, YYYY-MM, or YYYY-MM-DD)"),
		flags.WithStringFlag("to", "", "", "Show only changes up to and including this edition anchor (YYYY, YYYY-MM, or YYYY-MM-DD)"),
	)
}

// executeListEditions runs the list editions command.
func executeListEditions(cmd *cobra.Command, args []string) error {
	v := viper.GetViper()
	if err := editionsParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	opts := &EditionsOptions{
		Flags:  flags.ParseGlobalFlags(cmd, v),
		Format: v.GetString("format"),
		From:   v.GetString("from"),
		To:     v.GetString("to"),
	}

	return executeListEditionsWithOptions(opts)
}

func executeListEditionsWithOptions(opts *EditionsOptions) error {
	from, err := parseOptionalAnchor(opts.From)
	if err != nil {
		return err
	}
	to, err := parseOptionalAnchor(opts.To)
	if err != nil {
		return err
	}

	entries := edition.Between(from, to)
	if len(entries) == 0 {
		ui.Info("No default changes in the selected edition range")
		return nil
	}

	selector, err := column.NewSelector(editionColumns(), column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrCreateColumnSelector, err)
	}

	outputFormat := format.Format(opts.Format)
	// Old/New are historical values, not statuses — semantic true/false
	// coloring reads as pass/fail noise here. Instead, give the table calm
	// per-column contrast: dates and old values are muted (the past), the
	// key is accented like a command name (the identifier), and new values
	// get a subtle secondary accent (the present).
	r := renderer.New(nil, selector, nil, outputFormat, "",
		renderer.WithTableOptions(format.TableOptions{
			SemanticCellStyling: false,
			ColumnRoles: []format.ColumnRole{
				format.ColumnRoleMuted,      // Date.
				format.ColumnRoleIdentifier, // Key.
				format.ColumnRoleMuted,      // Old.
				format.ColumnRoleAccent,     // New.
				format.ColumnRoleNone,       // Description.
			},
		}))
	rows := editionsToData(entries)

	// For table format with TTY, add a footer summarizing the range. List
	// output always goes to the data channel (stdout) for pipeability.
	if outputFormat == "" || outputFormat == format.FormatTable {
		term := terminal.New()
		if term.IsTTY(terminal.Stdout) {
			output, err := r.RenderToString(rows)
			if err != nil {
				return err
			}
			return data.Write(output + buildEditionsFooter(len(entries), opts))
		}
	}

	return r.Render(rows)
}

// parseOptionalAnchor parses an anchor flag value, treating "" as unbounded.
func parseOptionalAnchor(raw string) (*edition.Anchor, error) {
	if raw == "" {
		return nil, nil
	}
	anchor, err := edition.ParseAnchor(raw)
	if err != nil {
		return nil, err
	}
	return &anchor, nil
}

// editionsToData converts journal entries to renderer rows, newest first.
func editionsToData(entries []edition.Entry) []map[string]any {
	data := make([]map[string]any, 0, len(entries))
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		data = append(data, map[string]any{
			"date":        entry.Date,
			"key":         entry.Key,
			"old":         fmt.Sprintf("%v", entry.Old),
			"new":         fmt.Sprintf("%v", entry.New),
			"description": entry.Description,
			"ref":         entry.Ref,
		})
	}
	return data
}

func editionColumns() []column.Config {
	return []column.Config{
		{Name: "Date", Value: "{{ .date }}"},
		{Name: "Key", Value: "{{ .key }}"},
		{Name: "Old", Value: "{{ .old }}"},
		{Name: "New", Value: "{{ .new }}"},
		{Name: "Description", Value: "{{ .description }}"},
	}
}

func buildEditionsFooter(count int, opts *EditionsOptions) string {
	styles := theme.GetCurrentStyles()

	footer := fmt.Sprintf("%d default change", count)
	if count != 1 {
		footer += "s"
	}
	switch {
	case opts.From != "" && opts.To != "":
		footer += fmt.Sprintf(" between editions %s and %s.", opts.From, opts.To)
	case opts.From != "":
		footer += fmt.Sprintf(" since edition %s. Pin edition: %q to keep the old defaults.", opts.From, opts.From)
	case opts.To != "":
		footer += fmt.Sprintf(" up to edition %s.", opts.To)
	default:
		footer += " journaled. Pin with `edition:` in atmos.yaml."
	}

	// Wrap to the terminal so the footer never overflows narrow windows.
	footer = ansi.Wrap(footer, templates.GetTerminalWidth(), "")
	return "\n" + styles.Footer.Render(footer) + "\n"
}
