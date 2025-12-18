package planfile

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/planfile"
	cfg "github.com/cloudposse/atmos/pkg/config"
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
	outputPath := ""
	if len(args) > 1 {
		outputPath = args[1]
	}

	// Default output path to basename of key.
	if outputPath == "" {
		outputPath = baseName(key)
	}

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
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

	// Download.
	ctx := context.Background()
	reader, metadata, err := store.Download(ctx, key)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Write to output file.
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("%w: failed to create output file %s: %v", errUtils.ErrPlanfileDownloadFailed, outputPath, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("%w: failed to write planfile: %v", errUtils.ErrPlanfileDownloadFailed, err)
	}

	_ = ui.Success(fmt.Sprintf("Downloaded planfile from %s: %s -> %s", store.Name(), key, outputPath))
	if metadata != nil && metadata.Stack != "" {
		_ = ui.Info(fmt.Sprintf("Stack: %s, Component: %s, SHA: %s", metadata.Stack, metadata.Component, metadata.SHA))
	}
	return nil
}

// baseName extracts the basename from a path/key.
func baseName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
