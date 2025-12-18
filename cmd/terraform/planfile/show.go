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

var showCmd = &cobra.Command{
	Use:   "show <key>",
	Short: "Show metadata for a Terraform plan file",
	Long:  `Show metadata for a Terraform plan file from the configured storage backend.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runShow,
}

var (
	showStore  string
	showFormat string
)

func init() {
	showCmd.Flags().StringVar(&showStore, "store", "", "Storage backend to use (default from config)")
	showCmd.Flags().StringVar(&showFormat, "format", "yaml", "Output format: json, yaml")
}

func runShow(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "planfile.runShow")()

	key := args[0]

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return err
	}

	// Get the storage configuration.
	storeOpts, err := getStoreOptions(&atmosConfig, showStore)
	if err != nil {
		return err
	}

	// Create the store.
	store, err := planfile.NewStore(storeOpts)
	if err != nil {
		return err
	}

	// Get metadata.
	ctx := context.Background()
	metadata, err := store.GetMetadata(ctx, key)
	if err != nil {
		return err
	}

	// Output based on format.
	switch showFormat {
	case "json":
		output, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal output: %w", err)
		}
		data.Writeln(string(output))

	default: // yaml format.
		data.Writef("key: %s\n", key)
		data.Writef("store: %s\n", store.Name())
		data.Writeln("metadata:")
		data.Writef("  stack: %s\n", metadata.Stack)
		data.Writef("  component: %s\n", metadata.Component)
		data.Writef("  component_path: %s\n", metadata.ComponentPath)
		data.Writef("  sha: %s\n", metadata.SHA)
		data.Writef("  base_sha: %s\n", metadata.BaseSHA)
		data.Writef("  branch: %s\n", metadata.Branch)
		data.Writef("  pr_number: %d\n", metadata.PRNumber)
		data.Writef("  run_id: %s\n", metadata.RunID)
		data.Writef("  repository: %s\n", metadata.Repository)
		data.Writef("  created_at: %s\n", metadata.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
		if metadata.ExpiresAt != nil {
			data.Writef("  expires_at: %s\n", metadata.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"))
		}
		data.Writef("  plan_summary: %s\n", metadata.PlanSummary)
		data.Writef("  has_changes: %t\n", metadata.HasChanges)
		data.Writef("  additions: %d\n", metadata.Additions)
		data.Writef("  changes: %d\n", metadata.Changes)
		data.Writef("  destructions: %d\n", metadata.Destructions)
	}

	return nil
}
