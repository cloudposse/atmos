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
	Key        string
	OutputPath string
}

var downloadCmd = &cobra.Command{
	Use:   "download <key> [output-path]",
	Short: "Download a Terraform plan file from storage",
	Long: `Download a Terraform plan file from the configured storage backend.

If output-path is not specified, the file is written to the current directory
with the basename of the key.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runDownload,
}

func init() {
	// Create parser with download-specific flags using functional options.
	downloadParser = flags.NewStandardParser(
		flags.WithStringFlag("store", "", "", "Storage backend to use (default from config)"),
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
	key := args[0]
	outputPath := getOutputPath(args)

	return &DownloadOptions{
		BaseOptions: parseBaseOptions(cmd, v),
		Key:         key,
		OutputPath:  outputPath,
	}
}

func runDownload(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "planfile.runDownload")()

	// Bind flags to Viper for proper precedence.
	v := viper.GetViper()
	if err := downloadParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Parse options.
	opts := parseDownloadOptions(cmd, v, args)

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

	// Download and write to file.
	metadata, err := downloadToFile(store, opts.Key, opts.OutputPath)
	if err != nil {
		return err
	}

	printDownloadSuccess(store.Name(), opts.Key, opts.OutputPath, metadata)
	return nil
}

// getOutputPath extracts output path from args or defaults to key basename.
func getOutputPath(args []string) string {
	if len(args) > 1 {
		return args[1]
	}
	return baseName(args[0])
}

// downloadToFile downloads the planfile and writes it to the output path.
func downloadToFile(store planfile.Store, key, outputPath string) (*planfile.Metadata, error) {
	ctx := context.Background()
	reader, metadata, err := store.Download(ctx, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	if err := writeToFile(outputPath, reader); err != nil {
		return nil, err
	}
	return metadata, nil
}

// writeToFile writes the reader contents to the output path.
func writeToFile(outputPath string, reader io.Reader) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("%w: failed to create output file %s: %w", errUtils.ErrPlanfileDownloadFailed, outputPath, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("%w: failed to write planfile to %s: %w", errUtils.ErrPlanfileDownloadFailed, outputPath, err)
	}
	return nil
}

// printDownloadSuccess prints the success message for a download.
func printDownloadSuccess(storeName, key, outputPath string, metadata *planfile.Metadata) {
	ui.Success(fmt.Sprintf("Downloaded planfile from %s: %s -> %s", storeName, key, outputPath))
	if metadata != nil && metadata.Stack != "" {
		ui.Info(fmt.Sprintf("Stack: %s, Component: %s, SHA: %s", metadata.Stack, metadata.Component, metadata.SHA))
	}
}

// baseName extracts the basename from a path/key.
func baseName(path string) string {
	return filepath.Base(path)
}
