package planfile

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// downloadParser handles flag parsing with Viper precedence for the download command.
var downloadParser *flags.StandardParser

// DownloadOptions contains parsed flags for the download command.
type DownloadOptions struct {
	BaseOptions
	Component  string
	OutputPath string
}

var downloadCmd = &cobra.Command{
	Use:   "download <component>",
	Short: "Download a Terraform plan file from storage",
	Long: `Download a Terraform plan file from the configured storage backend.

The component is specified as a positional argument and the stack via -s/--stack.
Use --output to specify the output path (defaults to plan.tfplan in current directory).`,
	Args: cobra.ExactArgs(1),
	RunE: runDownload,
}

func init() {
	// Create parser with download-specific flags using functional options.
	downloadParser = flags.NewStandardParser(
		flags.WithStringFlag("store", "", "", "Storage backend to use (default from config)"),
		flags.WithStringFlag("output", "o", planfile.PlanFilename, "Output path for the downloaded planfile"),
		flags.WithEnvVars("store", "ATMOS_PLANFILE_STORE"),
	)

	// Register flags with the command.
	downloadParser.RegisterFlags(downloadCmd)

	// Bind to Viper for environment variable support.
	if err := downloadParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add to parent command.
	PlanfileCmd.AddCommand(downloadCmd)
}

// parseDownloadOptions parses command flags into DownloadOptions.
func parseDownloadOptions(cmd *cobra.Command, v *viper.Viper, args []string) *DownloadOptions {
	return &DownloadOptions{
		BaseOptions: parseBaseOptions(cmd, v),
		Component:   args[0],
		OutputPath:  v.GetString("output"),
	}
}

func runDownload(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "planfile.runDownload")()

	// Bind flags to Viper for proper precedence.
	v := viper.GetViper()
	if err := downloadParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Bind persistent parent flags too.
	if err := planfileParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Parse options.
	opts := parseDownloadOptions(cmd, v, args)

	// Validate that stack is provided.
	if opts.Stack == "" {
		return fmt.Errorf("%w: --stack/-s is required for download", errUtils.ErrPlanfileStoreInvalidArgs)
	}

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

	// Resolve the planfile output path.
	// If --output was explicitly set, use that directly.
	// Otherwise, derive from component/stack via ProcessStacks.
	planfilePath, err := resolveDownloadPlanfilePath(cmd, opts, &atmosConfig)
	if err != nil {
		return err
	}

	// Create the store.
	store, err := createStore(&atmosConfig, opts.Store)
	if err != nil {
		return err
	}

	// Resolve SHA from context.
	resolved, err := resolveContext(false)
	if err != nil {
		return err
	}

	// Generate the key.
	key, err := resolveKey(opts.Component, opts.Stack, resolved.SHA)
	if err != nil {
		return err
	}

	// Download from store.
	ctx := context.Background()
	results, metadata, err := store.Download(ctx, key)
	if err != nil {
		return err
	}
	defer func() {
		for _, r := range results {
			r.Data.Close()
		}
	}()

	// Write downloaded files to disk.
	if err := planfile.WritePlanfileResults(results, planfilePath); err != nil {
		return err
	}

	printDownloadSuccess(store.Name(), key, planfilePath, metadata)
	return nil
}

// resolveDownloadPlanfilePath resolves the output planfile path.
// If --output was explicitly changed by the user, uses that value directly.
// Otherwise, derives the path from component/stack using ProcessStacks.
func resolveDownloadPlanfilePath(cmd *cobra.Command, opts *DownloadOptions, atmosConfig *schema.AtmosConfiguration) (string, error) {
	defer perf.Track(atmosConfig, "planfile.resolveDownloadPlanfilePath")()

	// If --output was explicitly set by the user, use it directly.
	if cmd.Flags().Changed("output") {
		return opts.OutputPath, nil
	}

	// Derive planfile path from component and stack.
	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: opts.Component,
		Stack:            opts.Stack,
		StackFromArg:     opts.Stack,
		ComponentType:    "terraform",
	}

	info, err := exec.ProcessStacks(atmosConfig, info, true, false, false, nil, nil)
	if err != nil {
		return "", fmt.Errorf("%w: failed to resolve component path: %w", errUtils.ErrPlanfileDownloadFailed, err)
	}

	planfilePath := exec.ConstructTerraformComponentPlanfilePath(atmosConfig, &info)
	return planfilePath, nil
}

// printDownloadSuccess prints the success message for a download.
func printDownloadSuccess(storeName, key, outputPath string, metadata *planfile.Metadata) {
	ui.Success(fmt.Sprintf("Downloaded planfile from %s: %s -> %s", storeName, key, outputPath))
	if metadata != nil && metadata.Stack != "" {
		ui.Info(fmt.Sprintf("Stack: %s, Component: %s, SHA: %s", metadata.Stack, metadata.Component, metadata.SHA))
	}
}
