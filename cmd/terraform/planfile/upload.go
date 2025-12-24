package planfile

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/planfile"

	// Import planfile store implementations to register them.
	_ "github.com/cloudposse/atmos/pkg/ci/planfile/github"
	_ "github.com/cloudposse/atmos/pkg/ci/planfile/local"
	_ "github.com/cloudposse/atmos/pkg/ci/planfile/s3"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// uploadParser handles flag parsing with Viper precedence for the upload command.
var uploadParser *flags.StandardParser

// UploadOptions contains parsed flags for the upload command.
type UploadOptions struct {
	BaseOptions
	PlanfilePath string
	Key          string
	Stack        string
	Component    string
	SHA          string
}

var uploadCmd = &cobra.Command{
	Use:   "upload <planfile>",
	Short: "Upload a Terraform plan file to storage",
	Long: `Upload a Terraform plan file to the configured storage backend.

The storage backend is configured in atmos.yaml under terraform.planfiles.
Supported backends: local, s3, github-artifacts.`,
	Args: cobra.ExactArgs(1),
	RunE: runUpload,
}

func init() {
	// Create parser with upload-specific flags using functional options.
	uploadParser = flags.NewStandardParser(
		flags.WithStringFlag("store", "", "", "Storage backend to use (default from config)"),
		flags.WithStringFlag("key", "", "", "Storage key (default: generated from stack/component/SHA)"),
		flags.WithStringFlag("stack", "", "", "Stack name for metadata"),
		flags.WithStringFlag("component", "", "", "Component name for metadata"),
		flags.WithStringFlag("sha", "", "", "Git SHA for metadata (default: current HEAD)"),
		flags.WithEnvVars("store", "ATMOS_PLANFILE_STORE"),
		flags.WithEnvVars("key", "ATMOS_PLANFILE_KEY"),
		flags.WithEnvVars("stack", "ATMOS_PLANFILE_STACK"),
		flags.WithEnvVars("component", "ATMOS_PLANFILE_COMPONENT"),
		flags.WithEnvVars("sha", "ATMOS_PLANFILE_SHA"),
	)

	// Register flags with the command.
	uploadParser.RegisterFlags(uploadCmd)

	// Bind to Viper for environment variable support.
	if err := uploadParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add to parent command.
	PlanfileCmd.AddCommand(uploadCmd)
}

// parseUploadOptions parses command flags into UploadOptions.
func parseUploadOptions(cmd *cobra.Command, v *viper.Viper, args []string) *UploadOptions {
	return &UploadOptions{
		BaseOptions:  parseBaseOptions(cmd, v),
		PlanfilePath: args[0],
		Key:          v.GetString("key"),
		Stack:        v.GetString("stack"),
		Component:    v.GetString("component"),
		SHA:          v.GetString("sha"),
	}
}

func runUpload(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "planfile.runUpload")()

	// Bind flags to Viper for proper precedence.
	v := viper.GetViper()
	if err := uploadParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Parse options.
	opts := parseUploadOptions(cmd, v, args)

	// Initialize configuration with global flags.
	atmosConfig, err := initAtmosConfig(opts)
	if err != nil {
		return err
	}

	// Create planfile store.
	store, err := createStore(&atmosConfig, opts.Store)
	if err != nil {
		return err
	}

	// Open the planfile.
	f, err := os.Open(opts.PlanfilePath)
	if err != nil {
		return fmt.Errorf("%w: failed to open planfile %s: %w", errUtils.ErrPlanfileUploadFailed, opts.PlanfilePath, err)
	}
	defer f.Close()

	// Build metadata and generate key.
	metadata := buildUploadMetadata(opts)
	key, err := resolveUploadKey(opts)
	if err != nil {
		return err
	}

	// Upload.
	ctx := context.Background()
	if err := store.Upload(ctx, key, f, metadata); err != nil {
		return err
	}

	_ = ui.Success(fmt.Sprintf("Uploaded planfile to %s: %s", store.Name(), key))
	return nil
}

// initAtmosConfig initializes Atmos configuration with global flags.
func initAtmosConfig(opts *UploadOptions) (schema.AtmosConfiguration, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath:           opts.BasePath,
		AtmosConfigFilesFromArg: opts.Config,
		AtmosConfigDirsFromArg:  opts.ConfigPath,
		ProfilesFromArg:         opts.Profile,
	}

	return cfg.InitCliConfig(configAndStacksInfo, true)
}

// createStore creates a planfile store from configuration.
func createStore(atmosConfig *schema.AtmosConfiguration, storeName string) (planfile.Store, error) {
	storeOpts, err := getStoreOptions(atmosConfig, storeName)
	if err != nil {
		return nil, err
	}
	return planfile.NewStore(storeOpts)
}

// buildUploadMetadata creates metadata for the planfile upload.
func buildUploadMetadata(opts *UploadOptions) *planfile.Metadata {
	return &planfile.Metadata{
		Stack:     opts.Stack,
		Component: opts.Component,
		SHA:       opts.SHA,
		CreatedAt: time.Now(),
	}
}

// resolveUploadKey returns the upload key, generating one if not provided.
func resolveUploadKey(opts *UploadOptions) (string, error) {
	if opts.Key != "" {
		return opts.Key, nil
	}
	keyPattern := planfile.DefaultKeyPattern()
	key, err := keyPattern.GenerateKey(&planfile.KeyContext{
		Stack:     opts.Stack,
		Component: opts.Component,
		SHA:       opts.SHA,
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate planfile key: %w", err)
	}
	return key, nil
}

// getStoreOptions builds StoreOptions from atmos configuration.
func getStoreOptions(atmosConfig *schema.AtmosConfiguration, storeName string) (planfile.StoreOptions, error) {
	defer perf.Track(atmosConfig, "planfile.getStoreOptions")()

	// For now, use defaults. In a full implementation, this would read from
	// atmosConfig.Terraform.Planfiles configuration.
	var storeType string
	var options map[string]any

	// If explicit store name provided, use it.
	if storeName != "" {
		storeType = storeName
		options = map[string]any{}
	}

	// Check environment for S3 configuration (only if not explicitly set).
	if storeType == "" {
		if bucket := os.Getenv("ATMOS_PLANFILE_BUCKET"); bucket != "" {
			storeType = "s3"
			options = map[string]any{
				"bucket": bucket,
				"prefix": os.Getenv("ATMOS_PLANFILE_PREFIX"),
				"region": os.Getenv("AWS_REGION"),
			}
		}
	}

	// Check environment for GitHub configuration (only if not explicitly set).
	if storeType == "" && os.Getenv("GITHUB_ACTIONS") == "true" {
		storeType = "github-artifacts"
		options = map[string]any{}
	}

	// Default to local storage.
	if storeType == "" {
		storeType = "local"
		options = map[string]any{
			"path": ".atmos/planfiles",
		}
	}

	return planfile.StoreOptions{
		Type:        storeType,
		Options:     options,
		AtmosConfig: atmosConfig,
	}, nil
}
