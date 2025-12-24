package planfile

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/ci/planfile"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// showParser handles flag parsing with Viper precedence for the show command.
var showParser *flags.StandardParser

// ShowOptions contains parsed flags for the show command.
type ShowOptions struct {
	BaseOptions
	Key    string
	Format string
}

var showCmd = &cobra.Command{
	Use:   "show <key>",
	Short: "Show metadata for a Terraform plan file",
	Long:  `Show metadata for a Terraform plan file from the configured storage backend.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runShow,
}

func init() {
	// Create parser with show-specific flags using functional options.
	showParser = flags.NewStandardParser(
		flags.WithStringFlag("store", "", "", "Storage backend to use (default from config)"),
		flags.WithStringFlag("format", "", "yaml", "Output format: json, yaml"),
		flags.WithEnvVars("store", "ATMOS_PLANFILE_STORE"),
		flags.WithEnvVars("format", "ATMOS_PLANFILE_SHOW_FORMAT"),
	)

	// Register flags with the command.
	showParser.RegisterFlags(showCmd)

	// Bind to Viper for environment variable support.
	if err := showParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add to parent command.
	PlanfileCmd.AddCommand(showCmd)
}

// parseShowOptions parses command flags into ShowOptions.
func parseShowOptions(cmd *cobra.Command, v *viper.Viper, args []string) *ShowOptions {
	return &ShowOptions{
		BaseOptions: parseBaseOptions(cmd, v),
		Key:         args[0],
		Format:      v.GetString("format"),
	}
}

func runShow(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "planfile.runShow")()

	// Bind flags to Viper for proper precedence.
	v := viper.GetViper()
	if err := showParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Parse options.
	opts := parseShowOptions(cmd, v, args)

	// Build ConfigAndStacksInfo from global flags to honor config selection flags.
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath:           opts.BasePath,
		AtmosConfigFilesFromArg: opts.Config,
		AtmosConfigDirsFromArg:  opts.ConfigPath,
		ProfilesFromArg:         opts.Profile,
	}

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return err
	}

	// Get the storage configuration.
	storeOpts, err := getStoreOptions(&atmosConfig, opts.Store)
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
	metadata, err := store.GetMetadata(ctx, opts.Key)
	if err != nil {
		return err
	}

	return formatShowOutput(opts.Key, store.Name(), metadata, opts.Format)
}

// formatShowOutput formats and outputs the planfile metadata in the specified format.
func formatShowOutput(key, storeName string, metadata *planfile.Metadata, format string) error {
	if format == "json" {
		output, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal output: %w", err)
		}
		_ = data.Writeln(string(output))
		return nil
	}

	// Default: yaml format.
	formatShowYAML(key, storeName, metadata)
	return nil
}

// formatShowYAML outputs the planfile metadata as YAML.
func formatShowYAML(key, storeName string, metadata *planfile.Metadata) {
	_ = data.Writef("key: %s\n", key)
	_ = data.Writef("store: %s\n", storeName)
	_ = data.Writeln("metadata:")
	_ = data.Writef("  stack: %s\n", metadata.Stack)
	_ = data.Writef("  component: %s\n", metadata.Component)
	_ = data.Writef("  component_path: %s\n", metadata.ComponentPath)
	_ = data.Writef("  sha: %s\n", metadata.SHA)
	_ = data.Writef("  base_sha: %s\n", metadata.BaseSHA)
	_ = data.Writef("  branch: %s\n", metadata.Branch)
	_ = data.Writef("  pr_number: %d\n", metadata.PRNumber)
	_ = data.Writef("  run_id: %s\n", metadata.RunID)
	_ = data.Writef("  repository: %s\n", metadata.Repository)
	_ = data.Writef("  created_at: %s\n", metadata.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	if metadata.ExpiresAt != nil {
		_ = data.Writef("  expires_at: %s\n", metadata.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"))
	}
	_ = data.Writef("  plan_summary: %s\n", metadata.PlanSummary)
	_ = data.Writef("  has_changes: %t\n", metadata.HasChanges)
	_ = data.Writef("  additions: %d\n", metadata.Additions)
	_ = data.Writef("  changes: %d\n", metadata.Changes)
	_ = data.Writef("  destructions: %d\n", metadata.Destructions)
}
