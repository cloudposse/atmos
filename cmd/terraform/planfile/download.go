package planfile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/planfile"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var downloadCmd = &cobra.Command{
	Use:   "download <key> [output-path]",
	Short: "Download a Terraform plan file from storage",
	Long: `Download a Terraform plan file from the configured storage backend.

If output-path is not specified, the file is written to the current directory
with the basename of the key.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runDownload,
}

var downloadStore string

func init() {
	downloadCmd.Flags().StringVar(&downloadStore, "store", "", "Storage backend to use (default from config)")
}

func runDownload(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "planfile.runDownload")()

	key := args[0]
	outputPath := getOutputPath(args)

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
	storeOpts, err := getStoreOptions(&atmosConfig, downloadStore)
	if err != nil {
		return err
	}

	// Create the store.
	store, err := planfile.NewStore(storeOpts)
	if err != nil {
		return err
	}

	// Download and write to file.
	metadata, err := downloadToFile(store, key, outputPath)
	if err != nil {
		return err
	}

	printDownloadSuccess(store.Name(), key, outputPath, metadata)
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
		return errors.Join(errUtils.ErrPlanfileDownloadFailed, fmt.Errorf("failed to create output file %s: %w", outputPath, err))
	}
	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return errors.Join(errUtils.ErrPlanfileDownloadFailed, fmt.Errorf("failed to write planfile: %w", err))
	}
	return nil
}

// printDownloadSuccess prints the success message for a download.
func printDownloadSuccess(storeName, key, outputPath string, metadata *planfile.Metadata) {
	_ = ui.Success(fmt.Sprintf("Downloaded planfile from %s: %s -> %s", storeName, key, outputPath))
	if metadata != nil && metadata.Stack != "" {
		_ = ui.Info(fmt.Sprintf("Stack: %s, Component: %s, SHA: %s", metadata.Stack, metadata.Component, metadata.SHA))
	}
}

// baseName extracts the basename from a path/key.
func baseName(path string) string {
	return filepath.Base(path)
}
