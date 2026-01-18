package list

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	aliasTypeBuiltIn    = "built-in"
	aliasTypeConfigured = "configured"
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
	Format  string
	Columns []string
	Sort    string
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
		"atmos list aliases --format yaml\n" +
		"atmos list aliases --columns alias,command\n" +
		"atmos list aliases --sort type:asc,alias:asc",
	Args: cobra.NoArgs,
	RunE: executeListAliases,
}

func init() {
	// Create parser with aliases-specific flags using flag wrappers.
	aliasesParser = NewListParser(
		WithFormatFlag,
		WithAliasesColumnsFlag,
		WithSortFlag,
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
		Flags:   flags.ParseGlobalFlags(cmd, v),
		Format:  v.GetString("format"),
		Columns: v.GetStringSlice("columns"),
		Sort:    v.GetString("sort"),
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
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrFailedToInitializeAtmosConfig, err)
	}

	// Collect all aliases.
	allAliases := collectAllAliases(rootCmd, atmosConfig.CommandAliases)

	if len(allAliases) == 0 {
		ui.Info("No aliases found")
		return nil
	}

	// Convert aliases to []map[string]any for renderer.
	data := aliasesToData(allAliases)

	// Build columns (use opts.Columns or default).
	columns := getAliasColumns(opts.Columns)

	// Build column selector.
	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrCreateColumnSelector, err)
	}

	// Build sorters.
	sorters, err := buildAliasSorters(opts.Sort)
	if err != nil {
		return err
	}

	// Determine output format.
	outputFormat := format.Format(opts.Format)

	// Create renderer.
	r := renderer.New(nil, selector, sorters, outputFormat, "")

	// For table format with TTY, use RenderToString and add footer.
	if outputFormat == "" || outputFormat == format.FormatTable {
		term := terminal.New()
		if term.IsTTY(terminal.Stdout) {
			// Render to string, add footer, then output.
			output, err := r.RenderToString(data)
			if err != nil {
				return err
			}
			footer := buildAliasFooter(allAliases)
			ui.Write(output + footer)
			return nil
		}
	}

	// For all other cases, use standard render.
	return r.Render(data)
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

// getAliasColumns returns column configs, using custom columns if specified.
func getAliasColumns(customColumns []string) []column.Config {
	// Default columns.
	defaultColumns := []column.Config{
		{Name: "Alias", Value: "{{ .alias }}"},
		{Name: "Command", Value: "{{ .command }}"},
		{Name: "Type", Value: "{{ .type }}"},
	}

	if len(customColumns) == 0 {
		return defaultColumns
	}

	// Map column names to configs.
	columnMap := map[string]column.Config{
		"alias":   {Name: "Alias", Value: "{{ .alias }}"},
		"command": {Name: "Command", Value: "{{ .command }}"},
		"type":    {Name: "Type", Value: "{{ .type }}"},
	}

	var result []column.Config
	for _, name := range customColumns {
		if col, ok := columnMap[strings.ToLower(name)]; ok {
			result = append(result, col)
		}
	}

	if len(result) == 0 {
		return defaultColumns
	}
	return result
}

// buildAliasSorters builds sorters from sort specification.
func buildAliasSorters(sortSpec string) ([]*listSort.Sorter, error) {
	if sortSpec == "" {
		// Default sort by alias name.
		return []*listSort.Sorter{listSort.NewSorter("Alias", listSort.Ascending)}, nil
	}
	return listSort.ParseSortSpec(sortSpec)
}

// buildAliasFooter builds the footer showing alias counts.
func buildAliasFooter(aliases []AliasInfo) string {
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

	return styles.Footer.Render(footer) + "\n"
}
