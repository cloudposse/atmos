package planfile

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
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

	// Download and write to file.
	metadata, err := downloadToFile(store, key, opts.OutputPath)
	if err != nil {
		return err
	}

	printDownloadSuccess(store.Name(), key, opts.OutputPath, metadata)
	return nil
}

// downloadToFile downloads the planfile and writes plan + lock to disk.
func downloadToFile(store planfile.Store, key, outputPath string) (*planfile.Metadata, error) {
	ctx := context.Background()
	results, metadata, err := store.Download(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, r := range results {
			r.Data.Close()
		}
	}()

	// Write each file to disk.
	for _, r := range results {
		var destPath string
		switch r.Name {
		case planfile.PlanFilename:
			destPath = outputPath
		case planfile.LockFilename:
			destPath = filepath.Join(filepath.Dir(outputPath), planfile.LockFilename)
		default:
			continue
		}

		fileData, err := io.ReadAll(r.Data)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to read %s: %w", errUtils.ErrPlanfileDownloadFailed, r.Name, err)
		}
		if err := os.WriteFile(destPath, fileData, 0o644); err != nil {
			return nil, fmt.Errorf("%w: failed to write %s to %s: %w", errUtils.ErrPlanfileDownloadFailed, r.Name, destPath, err)
		}
	}

	return metadata, nil
}

// printDownloadSuccess prints the success message for a download.
func printDownloadSuccess(storeName, key, outputPath string, metadata *planfile.Metadata) {
	ui.Success(fmt.Sprintf("Downloaded planfile from %s: %s -> %s", storeName, key, outputPath))
	if metadata != nil && metadata.Stack != "" {
		ui.Info(fmt.Sprintf("Stack: %s, Component: %s, SHA: %s", metadata.Stack, metadata.Component, metadata.SHA))
	}
}
