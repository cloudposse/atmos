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
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	aliasesHeaderSeparatorWidth = 50
	aliasTypeBuiltIn            = "built-in"
	aliasTypeConfigured         = "configured"
)

// AliasInfo represents a command alias with its source type.
type AliasInfo struct {
	Alias   string
	Command string
	Type    string // "built-in" or "configured"
}

var aliasesParser *flags.StandardParser

// AliasesOptions contains parsed flags for the aliases command.
type AliasesOptions struct {
	global.Flags
	Format string
}

// aliasesCmd lists configured command aliases.
var aliasesCmd = &cobra.Command{
	Use:   "aliases",
	Short: "List all command aliases (built-in and configured)",
	Long: `Display all command aliases including:
  - Built-in aliases: Native command shortcuts (e.g., tf → terraform)
  - Configured aliases: User-defined aliases from atmos.yaml`,
	Example: "atmos list aliases\n" +
		"atmos list aliases --format json\n" +
		"atmos list aliases --format yaml",
	Args: cobra.NoArgs,
	RunE: executeListAliases,
}

func init() {
	// Create parser with aliases-specific flags using flag wrappers.
	aliasesParser = NewListParser(
		WithFormatFlag,
	)

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
		Flags:  flags.ParseGlobalFlags(cmd, v),
		Format: v.GetString("format"),
	}

	// Get root command to collect built-in aliases.
	rootCmd := cmd.Root()

	return executeListAliasesWithOptions(opts, rootCmd)
}

func executeListAliasesWithOptions(opts *AliasesOptions, rootCmd *cobra.Command) error {
	// Load atmos configuration with global flags.
	configAndStacksInfo := buildConfigAndStacksInfo(&opts.Flags)
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return fmt.Errorf("failed to load atmos configuration: %w", err)
	}

	// Collect all aliases.
	allAliases := collectAllAliases(rootCmd, atmosConfig.CommandAliases)

	if len(allAliases) == 0 {
		ui.Info("No aliases found")
		return nil
	}

	// Use renderer for non-table formats (json, yaml, csv, tsv).
	if opts.Format != "" && opts.Format != "table" {
		return renderAliasesWithFormat(allAliases, opts.Format)
	}

	return displayAliases(allAliases)
}

// collectAllAliases gathers both built-in and configured aliases.
func collectAllAliases(rootCmd *cobra.Command, configuredAliases schema.CommandAliases) []AliasInfo {
	var allAliases []AliasInfo

	// Collect built-in aliases from command tree.
	builtInAliases := collectBuiltInAliases(rootCmd, "")
	allAliases = append(allAliases, builtInAliases...)

	// Collect configured aliases.
	for alias, command := range configuredAliases {
		allAliases = append(allAliases, AliasInfo{
			Alias:   alias,
			Command: command,
			Type:    aliasTypeConfigured,
		})
	}

	// Sort alphabetically by alias name for easy discovery.
	sort.Slice(allAliases, func(i, j int) bool {
		return allAliases[i].Alias < allAliases[j].Alias
	})

	return allAliases
}

// stripRootPrefix removes the root command name prefix from a path.
// Example: "atmos describe dependents" -> "describe dependents".
func stripRootPrefix(path, rootName string) string {
	prefix := rootName + " "
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return path
}

// collectCommandAliases collects aliases for a single command.
func collectCommandAliases(cmd *cobra.Command, parentPath, cmdPath, rootName string) []AliasInfo {
	var aliases []AliasInfo

	for _, alias := range cmd.Aliases {
		// Build the alias path (replace command name with alias in path).
		// For top-level commands: just the alias (e.g., "tf" for terraform).
		// For nested commands: parent path (without root) + alias (e.g., "describe dependants").
		aliasPath := alias
		if parentPath != rootName {
			parentPathWithoutRoot := stripRootPrefix(parentPath, rootName)
			aliasPath = parentPathWithoutRoot + " " + alias
		}

		// Build command path without root command name for display.
		displayCmdPath := stripRootPrefix(cmdPath, rootName)

		aliases = append(aliases, AliasInfo{
			Alias:   aliasPath,
			Command: displayCmdPath,
			Type:    aliasTypeBuiltIn,
		})
	}

	return aliases
}

// collectBuiltInAliases recursively collects Cobra command aliases from the command tree.
// It skips the root command name when building paths since aliases like "tf" should map
// to "terraform", not "atmos terraform".
func collectBuiltInAliases(cmd *cobra.Command, parentPath string) []AliasInfo {
	var aliases []AliasInfo

	// Build the full command path (excluding root command name for display).
	cmdPath := cmd.Name()
	if parentPath != "" {
		cmdPath = parentPath + " " + cmd.Name()
	}

	// Collect aliases for this command (skip root command itself).
	if parentPath != "" {
		rootName := cmd.Root().Name()
		aliases = collectCommandAliases(cmd, parentPath, cmdPath, rootName)
	}

	// Recursively collect from subcommands.
	for _, subCmd := range cmd.Commands() {
		// Skip help command and hidden commands.
		if subCmd.Name() == "help" || subCmd.Hidden {
			continue
		}
		subAliases := collectBuiltInAliases(subCmd, cmdPath)
		aliases = append(aliases, subAliases...)
	}

	return aliases
}

// renderAliasesWithFormat renders aliases using the list renderer infrastructure.
func renderAliasesWithFormat(aliases []AliasInfo, outputFormat string) error {
	// Convert aliases to []map[string]any for renderer.
	data := aliasesToData(aliases)

	// Define columns for aliases (including type).
	columns := []column.Config{
		{Name: "Alias", Value: "{{ .alias }}"},
		{Name: "Command", Value: "{{ .command }}"},
		{Name: "Type", Value: "{{ .type }}"},
	}

	// Build column selector.
	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("error creating column selector: %w", err)
	}

	// Create renderer with format and execute.
	r := renderer.New(nil, selector, nil, format.Format(outputFormat), "")

	return r.Render(data)
}

// aliasesToData converts AliasInfo slice to []map[string]any for the renderer.
func aliasesToData(aliases []AliasInfo) []map[string]any {
	data := make([]map[string]any, 0, len(aliases))
	for _, alias := range aliases {
		data = append(data, map[string]any{
			"alias":   alias.Alias,
			"command": alias.Command,
			"type":    alias.Type,
		})
	}
	return data
}

// displayAliases formats and displays the aliases to the terminal.
func displayAliases(aliases []AliasInfo) error {
	// Check if we're in TTY mode.
	if !term.IsTTYSupportForStdout() {
		// Fall back to simple text output for non-TTY.
		output := formatSimpleAliasesOutput(aliases)
		ui.Write(output)
		return nil
	}

	output := formatAliasesTable(aliases)
	ui.Write(output)
	return nil
}

// formatAliasesTable formats aliases into a styled Charmbracelet table.
func formatAliasesTable(aliases []AliasInfo) string {
	// Prepare headers and rows.
	headers := []string{"Alias", "Command", "Type"}
	var rows [][]string

	for _, alias := range aliases {
		row := []string{alias.Alias, alias.Command, alias.Type}
		rows = append(rows, row)
	}

	// Use minimal table with left-aligned columns (CreateThemedTable right-aligns column 0).
	output := theme.CreateMinimalTable(headers, rows) + "\n"

	// Footer message with breakdown.
	styles := theme.GetCurrentStyles()

	builtInCount := 0
	configuredCount := 0
	for _, alias := range aliases {
		if alias.Type == aliasTypeBuiltIn {
			builtInCount++
		} else {
			configuredCount++
		}
	}

	footer := fmt.Sprintf("\n%d alias", len(aliases))
	if len(aliases) != 1 {
		footer += "es"
	}
	footer += fmt.Sprintf(" (%d built-in, %d configured)", builtInCount, configuredCount)

	output += styles.Footer.Render(footer) + "\n"

	return output
}

// formatSimpleAliasesOutput formats aliases as simple text for non-TTY output.
func formatSimpleAliasesOutput(aliases []AliasInfo) string {
	var output string

	// Header.
	output += fmt.Sprintf("%-20s %-30s %s\n", "Alias", "Command", "Type")
	output += fmt.Sprintf("%s\n", strings.Repeat("=", aliasesHeaderSeparatorWidth+10))

	// Alias rows.
	for _, alias := range aliases {
		output += fmt.Sprintf("%-20s %-30s %s\n", alias.Alias, alias.Command, alias.Type)
	}

	// Footer message with breakdown.
	builtInCount := 0
	configuredCount := 0
	for _, alias := range aliases {
		if alias.Type == aliasTypeBuiltIn {
			builtInCount++
		} else {
			configuredCount++
		}
	}

	output += fmt.Sprintf("\n%d alias", len(aliases))
	if len(aliases) != 1 {
		output += "es"
	}
	output += fmt.Sprintf(" (%d built-in, %d configured)\n", builtInCount, configuredCount)

	return output
}
