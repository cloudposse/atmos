package workdir

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

var listParser *flags.StandardParser

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all workdirs",
	Long:    `Show all component working directories in the project.`,
	Example: `  atmos terraform workdir list`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "workdir.list.RunE")()

		v := viper.GetViper()
		format := v.GetString("format")

		// Initialize config.
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return errUtils.Build(errUtils.ErrWorkdirMetadata).
				WithCause(err).
				WithExplanation("Failed to load atmos configuration").
				Err()
		}

		// Get workdirs.
		workdirs, err := workdirManager.ListWorkdirs(&atmosConfig)
		if err != nil {
			return err
		}

		// Output based on format.
		switch format {
		case "json":
			return printListJSON(workdirs)
		case "yaml":
			return printListYAML(workdirs)
		default:
			printListTable(workdirs)
			return nil
		}
	},
}

func printListJSON(workdirs []WorkdirInfo) error {
	jsonData, err := json.MarshalIndent(workdirs, "", "  ")
	if err != nil {
		return errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("Failed to marshal workdirs to JSON").
			Err()
	}
	fmt.Println(string(jsonData))
	return nil
}

func printListYAML(workdirs []WorkdirInfo) error {
	yamlData, err := yaml.Marshal(workdirs)
	if err != nil {
		return errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("Failed to marshal workdirs to YAML").
			Err()
	}
	fmt.Print(string(yamlData))
	return nil
}

func printListTable(workdirs []WorkdirInfo) {
	if len(workdirs) == 0 {
		fmt.Fprintln(os.Stderr, "No workdirs found")
		return
	}

	// Build rows.
	var rows [][]string
	for i := range workdirs {
		rows = append(rows, []string{
			workdirs[i].Component,
			workdirs[i].Stack,
			workdirs[i].Source,
			workdirs[i].CreatedAt.Format("2006-01-02 15:04"),
			workdirs[i].Path,
		})
	}

	// Create table with lipgloss.
	headers := []string{"COMPONENT", "STACK", "SOURCE", "CREATED", "PATH"}

	t := table.New().
		Headers(headers...).
		Rows(rows...).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderRow(false).
		BorderColumn(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().
					Foreground(lipgloss.Color(theme.ColorCyan)).
					Bold(true).
					Padding(0, 2, 0, 0)
			}
			return lipgloss.NewStyle().Padding(0, 2, 0, 0)
		})

	fmt.Fprintln(os.Stderr, t)
}

func init() {
	listCmd.DisableFlagParsing = false

	// Create parser with functional options.
	listParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "f", "table", "Output format: table, yaml, json"),
		flags.WithEnvVars("format", "ATMOS_FORMAT"),
	)

	// Register flags with the command.
	listParser.RegisterFlags(listCmd)

	// Bind flags to Viper.
	if err := listParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
