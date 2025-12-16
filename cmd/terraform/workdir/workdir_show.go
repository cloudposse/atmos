package workdir

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

var showParser *flags.StandardParser

var showCmd = &cobra.Command{
	Use:   "show <component>",
	Short: "Show workdir details",
	Long: `Display detailed information about a component's working directory.

The output is formatted for human readability, similar to 'kubectl describe'.`,
	Example: `  atmos terraform workdir show vpc --stack dev`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "workdir.show.RunE")()

		component := args[0]

		v := viper.GetViper()
		stack := v.GetString("stack")

		if stack == "" {
			return errUtils.Build(errUtils.ErrWorkdirMetadata).
				WithExplanation("Stack is required").
				WithHint("Use --stack to specify the stack").
				Err()
		}

		// Initialize config.
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return errUtils.Build(errUtils.ErrWorkdirMetadata).
				WithCause(err).
				WithExplanation("Failed to load atmos configuration").
				Err()
		}

		// Get workdir info.
		info, err := workdirManager.GetWorkdirInfo(&atmosConfig, component, stack)
		if err != nil {
			return err
		}

		// Print in auth whoami style.
		printShowHuman(info)
		return nil
	},
}

func printShowHuman(info *WorkdirInfo) {
	// Display status indicator with colored checkmark.
	statusIndicator := theme.Styles.Checkmark.String()
	fmt.Fprintf(os.Stderr, "%s Workdir Status\n\n", statusIndicator)

	// Build table rows.
	rows := [][]string{
		{"Name", info.Name},
		{"Component", info.Component},
		{"Stack", info.Stack},
		{"Source", info.Source},
		{"Path", info.Path},
	}

	if info.ContentHash != "" {
		rows = append(rows, []string{"Content Hash", info.ContentHash})
	}

	rows = append(rows, []string{"Created", info.CreatedAt.Format("2006-01-02 15:04:05 MST")})
	rows = append(rows, []string{"Updated", info.UpdatedAt.Format("2006-01-02 15:04:05 MST")})

	// Create table with lipgloss (like auth whoami).
	t := table.New().
		Rows(rows...).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderRow(false).
		BorderColumn(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			if col == 0 {
				return lipgloss.NewStyle().
					Foreground(lipgloss.Color(theme.ColorCyan)).
					Padding(0, 1, 0, 2)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})

	fmt.Fprintf(os.Stderr, "%s\n", t)
}

func init() {
	showCmd.DisableFlagParsing = false

	// Create parser with functional options.
	showParser = flags.NewStandardParser(
		flags.WithStackFlag(),
	)

	// Register flags with the command.
	showParser.RegisterFlags(showCmd)

	// Bind flags to Viper.
	if err := showParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
