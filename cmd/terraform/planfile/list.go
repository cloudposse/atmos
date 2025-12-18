package planfile

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/ci/planfile"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
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

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
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

	// Output based on format.
	switch listFormat {
	case "json":
		output, err := json.MarshalIndent(files, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal output: %w", err)
		}
		data.Writeln(string(output))

	case "yaml":
		// Simple YAML output.
		for _, f := range files {
			data.Writef("- key: %s\n", f.Key)
			data.Writef("  size: %d\n", f.Size)
			data.Writef("  last_modified: %s\n", f.LastModified.Format("2006-01-02T15:04:05Z07:00"))
			if f.Metadata != nil {
				data.Writef("  stack: %s\n", f.Metadata.Stack)
				data.Writef("  component: %s\n", f.Metadata.Component)
				data.Writef("  sha: %s\n", f.Metadata.SHA)
			}
		}

	default: // table format.
		if len(files) == 0 {
			data.Writeln("No planfiles found.")
			return nil
		}

		// Print header.
		data.Writef("%-60s %10s %s\n", "KEY", "SIZE", "LAST MODIFIED")
		data.Writeln(repeatStr("-", 100))

		for _, f := range files {
			data.Writef("%-60s %10d %s\n", f.Key, f.Size, f.LastModified.Format("2006-01-02 15:04"))
		}

		data.Writef("\nTotal: %d planfile(s)\n", len(files))
	}

	return nil
}

// repeatStr repeats a string n times.
func repeatStr(s string, n int) string {
	result := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		result = append(result, s...)
	}
	return string(result)
}
