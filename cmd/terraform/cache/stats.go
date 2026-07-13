package cache

import (
	"fmt"

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

var statsParser *flags.StandardParser

// statsCmd reports filesystem facts about the cache. It deliberately does NOT
// report a hit rate: there is no persistent hit/miss store (the filesystem is the
// index). Per-run hit statistics are surfaced by the savings report at the end of a
// terraform run; persistent hit-rate reporting would require the future, optional
// metrics database and is intentionally out of scope.
var statsCmd = &cobra.Command{
	Use:     "stats",
	Short:   "Show cache size and object counts",
	Long:    `Report filesystem facts about the cache: total size, object count, and provider/module breakdown.`,
	Example: `  atmos terraform cache stats`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		defer perf.Track(atmosConfigPtr, "cache.stats.RunE")()

		v := viper.GetViper()
		if err := statsParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		format := v.GetString("format")

		root, err := resolveCacheRoot(cmd)
		if err != nil {
			return err
		}
		summary, err := tfcache.Summarize(root)
		if err != nil {
			return err
		}

		return renderFormatted(format, summary, func() { printStats(summary) })
	},
}

func printStats(s tfcache.Summary) {
	rows := [][]string{
		{"Registry cache root", s.Root},
		{"Objects", fmt.Sprintf("%d (%d providers, %d modules)", s.ObjectCount, s.Providers, s.Modules)},
	}
	//nolint:gosec // total size is non-negative.
	rows = append(rows, []string{"Total size", humanize.Bytes(uint64(s.TotalSize))})
	if s.Largest != nil {
		//nolint:gosec // object size is non-negative.
		rows = append(rows, []string{"Largest", fmt.Sprintf("%s (%s)", s.Largest.Key, humanize.Bytes(uint64(s.Largest.Size)))})
	}
	if s.Oldest != nil && !s.Oldest.ModTime.IsZero() {
		rows = append(rows, []string{"Oldest", fmt.Sprintf("%s (%s)", s.Oldest.Key, humanize.Time(s.Oldest.ModTime))})
	}

	t := table.New().
		Headers("METRIC", "VALUE").
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
	statsParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "f", "table", "Output format: table, yaml, json"),
		flags.WithEnvVars("format", "ATMOS_FORMAT"),
	)
	statsParser.RegisterFlags(statsCmd)
	if err := statsParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
