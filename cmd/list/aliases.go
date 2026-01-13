package list

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	aliasesHeaderSeparatorWidth = 50
)

var aliasesParser *flags.StandardParser

// AliasesOptions contains parsed flags for the aliases command.
type AliasesOptions struct {
	global.Flags
}

// aliasesCmd lists configured command aliases.
var aliasesCmd = &cobra.Command{
	Use:   "aliases",
	Short: "List configured command aliases",
	Long:  "Display all command aliases configured in atmos.yaml.",
	Example: "atmos list aliases\n" +
		"atmos list aliases --format json",
	Args: cobra.NoArgs,
	RunE: executeListAliases,
}

func init() {
	// Create parser with aliases-specific flags using functional options.
	aliasesParser = flags.NewStandardParser()

	// Register flags.
	aliasesParser.RegisterFlags(aliasesCmd)

	// Bind flags to Viper for environment variable support.
	if err := aliasesParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

// executeListAliases runs the list aliases command.
func executeListAliases(cmd *cobra.Command, args []string) error {
	// Parse flags using StandardParser with Viper precedence.
	v := viper.GetViper()
	if err := aliasesParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	opts := &AliasesOptions{
		Flags: flags.ParseGlobalFlags(cmd, v),
	}

	return executeListAliasesWithOptions(opts)
}

func executeListAliasesWithOptions(_ *AliasesOptions) error {
	// Load atmos configuration.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return fmt.Errorf("failed to load atmos configuration: %w", err)
	}

	aliases := atmosConfig.CommandAliases
	if len(aliases) == 0 {
		_ = ui.Info("No aliases configured")
		return nil
	}

	return displayAliases(aliases)
}

// displayAliases formats and displays the aliases to the terminal.
func displayAliases(aliases schema.CommandAliases) error {
	// Check if we're in TTY mode.
	if !term.IsTTYSupportForStdout() {
		// Fall back to simple text output for non-TTY.
		output := formatSimpleAliasesOutput(aliases)
		return ui.Write(output)
	}

	output := formatAliasesTable(aliases)
	return ui.Write(output)
}

// formatAliasesTable formats aliases into a styled Charmbracelet table.
func formatAliasesTable(aliases schema.CommandAliases) string {
	// Prepare headers and rows.
	headers := []string{"Alias", "Command"}
	var rows [][]string

	// Sort aliases by name for consistent output.
	sortedNames := make([]string, 0, len(aliases))
	for name := range aliases {
		sortedNames = append(sortedNames, name)
	}
	sort.Strings(sortedNames)

	for _, name := range sortedNames {
		command := aliases[name]
		row := []string{name, command}
		rows = append(rows, row)
	}

	// Use the themed table creation.
	output := theme.CreateThemedTable(headers, rows) + "\n"

	// Footer message.
	styles := theme.GetCurrentStyles()

	footer := fmt.Sprintf("\n%d alias", len(aliases))
	if len(aliases) != 1 {
		footer += "es"
	}
	footer += " configured."

	output += styles.Footer.Render(footer) + "\n"

	return output
}

// formatSimpleAliasesOutput formats aliases as simple text for non-TTY output.
func formatSimpleAliasesOutput(aliases schema.CommandAliases) string {
	var output string

	// Header.
	output += fmt.Sprintf("%-15s %s\n", "Alias", "Command")
	output += fmt.Sprintf("%s\n", strings.Repeat("=", aliasesHeaderSeparatorWidth))

	// Sort aliases by name for consistent output.
	sortedNames := make([]string, 0, len(aliases))
	for name := range aliases {
		sortedNames = append(sortedNames, name)
	}
	sort.Strings(sortedNames)

	// Alias rows.
	for _, name := range sortedNames {
		command := aliases[name]
		output += fmt.Sprintf("%-15s %s\n", name, command)
	}

	// Footer message.
	output += fmt.Sprintf("\n%d alias", len(aliases))
	if len(aliases) != 1 {
		output += "es"
	}
	output += " configured.\n"

	return output
}
