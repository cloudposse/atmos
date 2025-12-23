package planfile

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/ci/planfile"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// TableHeaderWidth is the width for the table header separator line.
	tableHeaderWidth = 100
)

var listCmd = &cobra.Command{
	Use:   "list [prefix]",
	Short: "List Terraform plan files in storage",
	Long: `List Terraform plan files from the configured storage backend.

Optionally filter by prefix (e.g., stack name or component).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runList,
}

var (
	listStore  string
	listFormat string
)

func init() {
	listCmd.Flags().StringVar(&listStore, "store", "", "Storage backend to use (default from config)")
	listCmd.Flags().StringVar(&listFormat, "format", "table", "Output format: table, json, yaml")
}

func runList(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "planfile.runList")()

	prefix := ""
	if len(args) > 0 {
		prefix = args[0]
	}

	// Get global flags from Viper (includes base-path, config, config-path, profile).
	v := viper.GetViper()
	globalFlags := flags.ParseGlobalFlags(cmd, v)

	// Build ConfigAndStacksInfo from global flags to honor config selection flags.
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		ProfilesFromArg:         globalFlags.Profile,
	}

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return err
	}

	// Get the storage configuration.
	storeOpts, err := getStoreOptions(&atmosConfig, listStore)
	if err != nil {
		return err
	}

	// Create the store.
	store, err := planfile.NewStore(storeOpts)
	if err != nil {
		return err
	}

	// List planfiles.
	ctx := context.Background()
	files, err := store.List(ctx, prefix)
	if err != nil {
		return err
	}

	return formatListOutput(files, listFormat)
}

// formatListOutput formats and outputs the planfile list in the specified format.
func formatListOutput(files []planfile.PlanfileInfo, format string) error {
	switch format {
	case "json":
		return formatListJSON(files)
	case "yaml":
		formatListYAML(files)
	default:
		formatListTable(files)
	}
	return nil
}

// formatListJSON outputs the planfile list as JSON.
func formatListJSON(files []planfile.PlanfileInfo) error {
	output, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal output: %w", err)
	}
	_ = data.Writeln(string(output))
	return nil
}

// formatListYAML outputs the planfile list as YAML.
func formatListYAML(files []planfile.PlanfileInfo) {
	for _, f := range files {
		_ = data.Writef("- key: %s\n", f.Key)
		_ = data.Writef("  size: %d\n", f.Size)
		_ = data.Writef("  last_modified: %s\n", f.LastModified.Format("2006-01-02T15:04:05Z07:00"))
		if f.Metadata != nil {
			_ = data.Writef("  stack: %s\n", f.Metadata.Stack)
			_ = data.Writef("  component: %s\n", f.Metadata.Component)
			_ = data.Writef("  sha: %s\n", f.Metadata.SHA)
		}
	}
}

// formatListTable outputs the planfile list as a table.
func formatListTable(files []planfile.PlanfileInfo) {
	if len(files) == 0 {
		_ = data.Writeln("No planfiles found.")
		return
	}

	_ = data.Writef("%-60s %10s %s\n", "KEY", "SIZE", "LAST MODIFIED")
	_ = data.Writeln(strings.Repeat("-", tableHeaderWidth))

	for _, f := range files {
		_ = data.Writef("%-60s %10d %s\n", f.Key, f.Size, f.LastModified.Format("2006-01-02 15:04"))
	}

	_ = data.Writef("\nTotal: %d planfile(s)\n", len(files))
}
