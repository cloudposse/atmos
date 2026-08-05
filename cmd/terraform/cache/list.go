package cache

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	tfcache "github.com/cloudposse/atmos/pkg/terraform/cache"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

var listParser *flags.StandardParser

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List cached providers and modules",
	Long:    `Show every cached Terraform provider and module object.`,
	Example: `  atmos terraform cache list`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		defer perf.Track(atmosConfigPtr, "cache.list.RunE")()

		v := viper.GetViper()
		if err := listParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		format := v.GetString("format")

		root, err := resolveCacheRoot(cmd)
		if err != nil {
			return err
		}
		entries, err := tfcache.List(root)
		if err != nil {
			return err
		}

		return renderFormatted(format, entries, func() { printListTable(entries) })
	},
}

func printListTable(entries []tfcache.Entry) {
	if len(entries) == 0 {
		ui.Writeln("No cached artifacts found")
		return
	}

	var rows [][]string
	for i := range entries {
		e := entries[i]
		age := "-"
		if !e.ModTime.IsZero() {
			age = humanize.Time(e.ModTime)
		}
		//nolint:gosec // object size is non-negative.
		rows = append(rows, []string{e.Key, e.Group, e.Kind, humanize.Bytes(uint64(e.Size)), age})
	}

	t := table.New().
		Headers("KEY", "GROUP", "KIND", "SIZE", "AGE").
		Rows(rows...).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderRow(false).BorderColumn(false).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan)).Bold(true).Padding(0, 2, 0, 0)
			}
			return lipgloss.NewStyle().Padding(0, 2, 0, 0)
		})

	ui.Writeln(t.String())
}

func init() {
	listParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "f", "table", "Output format: table, yaml, json"),
		flags.WithEnvVars("format", "ATMOS_FORMAT"),
	)
	listParser.RegisterFlags(listCmd)
	if err := listParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
