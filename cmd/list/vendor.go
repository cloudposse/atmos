package list

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	l "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/list/column"
	"github.com/cloudposse/atmos/pkg/list/extract"
	"github.com/cloudposse/atmos/pkg/list/filter"
	"github.com/cloudposse/atmos/pkg/list/format"
	"github.com/cloudposse/atmos/pkg/list/renderer"
	listSort "github.com/cloudposse/atmos/pkg/list/sort"
	perf "github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var vendorParser *flags.StandardParser

// VendorOptions contains parsed flags for the vendor command.
type VendorOptions struct {
	global.Flags
	Format  string
	Stack   string
	Columns []string
	Sort    string
}

// vendorCmd lists vendor configurations.
var vendorCmd = &cobra.Command{
	Use:   "vendor",
	Short: "List all vendor configurations with filtering, sorting, and formatting options",
	Long:  `List Atmos vendor configurations including component and vendor manifests with support for filtering, custom column selection, sorting, and multiple output formats.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get Viper instance for flag/env precedence.
		v := viper.GetViper()

		// Skip stack validation for vendor (honors --base-path, --config, --config-path, --profile).
		if err := checkAtmosConfig(cmd, v, true); err != nil {
			return err
		}

		// Parse flags using StandardParser with Viper precedence.
		if err := vendorParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &VendorOptions{
			Flags:   flags.ParseGlobalFlags(cmd, v),
			Format:  v.GetString("format"),
			Stack:   v.GetString("stack"),
			Columns: v.GetStringSlice("columns"),
			Sort:    v.GetString("sort"),
		}

		return listVendorWithOptions(opts)
	},
}

// columnsCompletionForVendor provides dynamic tab completion for --columns flag.
// Returns column names from atmos.yaml vendor.list.columns configuration.
func columnsCompletionForVendor(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "list.vendor.columnsCompletionForVendor")()

	// Load atmos configuration.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Extract column names from atmos.yaml configuration.
	if len(atmosConfig.Vendor.List.Columns) > 0 {
		var columnNames []string
		for _, col := range atmosConfig.Vendor.List.Columns {
			columnNames = append(columnNames, col.Name)
		}
		return columnNames, cobra.ShellCompDirectiveNoFileComp
	}

	// If no custom columns configured, return empty list.
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	// Create parser with vendor-specific flags using flag wrappers.
	vendorParser = NewListParser(
		WithFormatFlag,
		WithVendorColumnsFlag,
		WithSortFlag,
		WithStackFlag,
	)

	// Register flags.
	vendorParser.RegisterFlags(vendorCmd)

	// Register dynamic tab completion for --columns flag.
	if err := vendorCmd.RegisterFlagCompletionFunc("columns", columnsCompletionForVendor); err != nil {
		panic(err)
	}

	// Add stack completion.
	addStackCompletion(vendorCmd)

	// Bind flags to Viper for environment variable support.
	if err := vendorParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func listVendorWithOptions(opts *VendorOptions) error {
	defer perf.Track(nil, "list.vendor.listVendorWithOptions")()

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := config.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return err
	}

	// If format is empty, check command-specific config.
	if opts.Format == "" && atmosConfig.Vendor.List.Format != "" {
		opts.Format = atmosConfig.Vendor.List.Format
	}

	// Get vendor configurations.
	vendorInfos, err := l.GetVendorInfos(&atmosConfig)
	if err != nil {
		return err
	}

	// Convert to renderer-compatible format.
	vendors, err := extract.Vendor(vendorInfos)
	if err != nil {
		return err
	}

	if len(vendors) == 0 {
		_ = ui.Info("No vendor configurations found")
		return nil
	}

	// Build filters.
	filters := buildVendorFilters(opts)

	// Get column configuration.
	columns := getVendorColumns(&atmosConfig, opts.Columns)

	// Build column selector.
	selector, err := column.NewSelector(columns, column.BuildColumnFuncMap())
	if err != nil {
		return fmt.Errorf("error creating column selector: %w", err)
	}

	// Build sorters.
	sorters, err := buildVendorSorters(opts.Sort)
	if err != nil {
		return fmt.Errorf("error parsing sort specification: %w", err)
	}

	// Create renderer and execute pipeline.
	outputFormat := format.Format(opts.Format)
	r := renderer.New(filters, selector, sorters, outputFormat, "")

	return r.Render(vendors)
}

// buildVendorFilters creates filters based on command options.
func buildVendorFilters(opts *VendorOptions) []filter.Filter {
	defer perf.Track(nil, "list.vendor.buildVendorFilters")()

	var filters []filter.Filter

	// Component filter (glob pattern on component field).
	// Vendor rows contain: component, type, manifest, folder.
	if opts.Stack != "" {
		globFilter, err := filter.NewGlobFilter("component", opts.Stack)
		if err == nil {
			filters = append(filters, globFilter)
		}
	}

	return filters
}

// getVendorColumns returns column configuration.
func getVendorColumns(atmosConfig *schema.AtmosConfiguration, columnsFlag []string) []column.Config {
	defer perf.Track(nil, "list.vendor.getVendorColumns")()

	// If --columns flag is provided, parse it and return.
	if len(columnsFlag) > 0 {
		return parseColumnsFlag(columnsFlag)
	}

	// Check atmos.yaml for vendor.list.columns configuration.
	if len(atmosConfig.Vendor.List.Columns) > 0 {
		var configs []column.Config
		for _, col := range atmosConfig.Vendor.List.Columns {
			configs = append(configs, column.Config{
				Name:  col.Name,
				Value: col.Value,
				Width: col.Width,
			})
		}
		return configs
	}

	// Default columns for vendor.
	return []column.Config{
		{Name: "Component", Value: "{{ .component }}"},
		{Name: "Type", Value: "{{ .type }}"},
		{Name: "Tags", Value: "{{ .tags }}"},
		{Name: "Manifest", Value: "{{ .manifest }}"},
		{Name: "Folder", Value: "{{ .folder }}"},
	}
}

// buildVendorSorters creates sorters from sort specification.
func buildVendorSorters(sortSpec string) ([]*listSort.Sorter, error) {
	defer perf.Track(nil, "list.vendor.buildVendorSorters")()

	if sortSpec == "" {
		// Default sort: by component ascending.
		return []*listSort.Sorter{
			listSort.NewSorter("Component", listSort.Ascending),
		}, nil
	}

	return listSort.ParseSortSpec(sortSpec)
}

// obfuscateHomeDirInOutput replaces occurrences of the home directory with "~" to prevent leaking user paths.
func obfuscateHomeDirInOutput(output string) string {
	homeDir, err := homedir.Dir()
	if err != nil || homeDir == "" {
		return output
	}

	// Replace home directory with tilde only at path boundaries.
	// This prevents replacing homeDir when it's a prefix of another directory name.
	// For example, if homeDir is "/home/user", we should not replace "/home/username".
	sep := string(os.PathSeparator)

	// First replace homeDir followed by separator (e.g., "/home/user/file" -> "~/file").
	result := strings.ReplaceAll(output, homeDir+sep, "~"+sep)

	// Then replace homeDir at the end of string or followed by non-path characters.
	// We need to handle cases like:
	// - homeDir alone (e.g., "/home/user" -> "~")
	// - homeDir followed by space, newline, or other delimiters (e.g., "/home/user\n" -> "~\n")
	// But NOT homeDir as a prefix (e.g., "/home/username" should remain unchanged).
	var builder strings.Builder
	builder.Grow(len(result))

	i := 0
	for i < len(result) {
		// Check if we have homeDir at current position.
		if strings.HasPrefix(result[i:], homeDir) {
			nextPos := i + len(homeDir)
			if shouldReplaceHomeDir(result, nextPos) {
				builder.WriteString("~")
				i = nextPos
			} else {
				// This is a prefix - keep original.
				builder.WriteString(homeDir)
				i = nextPos
			}
		} else {
			builder.WriteByte(result[i])
			i++
		}
	}

	return builder.String()
}

// shouldReplaceHomeDir checks if homeDir at the current position should be replaced with ~.
// Returns true if at end of string or followed by non-path characters.
// Returns false if followed by alphanumeric/dash/underscore (indicating a prefix).
func shouldReplaceHomeDir(result string, nextPos int) bool {
	// homeDir at end of string - replace it.
	if nextPos >= len(result) {
		return true
	}

	nextChar := result[nextPos]
	// Check if next character is alphanumeric/dash/underscore.
	// If so, this is likely a prefix of another name - don't replace.
	if isPathChar(nextChar) {
		return false
	}

	// This is a boundary - replace with ~.
	return true
}

// isPathChar returns true if the character is typically part of a path component name.
func isPathChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_'
}
