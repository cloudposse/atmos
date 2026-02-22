package planfile

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"

	// Import planfile store implementations to register them.
	_ "github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile/github"
	_ "github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile/local"
	_ "github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile/s3"
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
	Use:   "upload [options]",
	Short: "Upload a Terraform plan file to storage",
	Long: `Upload a Terraform plan file to the configured storage backend.

The storage backend is configured in atmos.yaml under terraform.planfiles.
Supported backends: local, s3, github-artifacts.

When --planfile is omitted, the planfile path is derived from --component and --stack.`,
	Args: cobra.NoArgs,
	RunE: runUpload,
}

func init() {
	// Create parser with upload-specific flags using functional options.
	uploadParser = flags.NewStandardParser(
		flags.WithStringFlag("store", "", "", "Storage backend to use (default from config)"),
		flags.WithStringFlag("planfile", "", "", "Path to the planfile to upload (default: derived from component/stack)"),
		flags.WithStringFlag("key", "", "", "Storage key (default: generated from stack/component/SHA)"),
		flags.WithStringFlag("stack", "", "", "Stack name for metadata"),
		flags.WithStringFlag("component", "", "", "Component name for metadata"),
		flags.WithStringFlag("sha", "", "", "Git SHA for metadata (default: current HEAD)"),
		flags.WithEnvVars("store", "ATMOS_PLANFILE_STORE"),
		flags.WithEnvVars("planfile", "ATMOS_PLANFILE_PATH"),
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
func parseUploadOptions(cmd *cobra.Command, v *viper.Viper) *UploadOptions {
	return &UploadOptions{
		BaseOptions:  parseBaseOptions(cmd, v),
		PlanfilePath: v.GetString("planfile"),
		Key:          v.GetString("key"),
		Stack:        v.GetString("stack"),
		Component:    v.GetString("component"),
		SHA:          v.GetString("sha"),
	}
}

func runUpload(cmd *cobra.Command, _ []string) error {
	defer perf.Track(nil, "planfile.runUpload")()

	// Bind flags to Viper for proper precedence.
	v := viper.GetViper()
	if err := uploadParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Parse options.
	opts := parseUploadOptions(cmd, v)

	// Initialize configuration with global flags.
	atmosConfig, err := initAtmosConfig(opts)
	if err != nil {
		return err
	}

	// Resolve planfile path (from flag or derived from component/stack).
	planfilePath, err := resolveUploadPlanfilePath(opts, &atmosConfig)
	if err != nil {
		return err
	}

	// Create planfile store.
	store, err := createStore(&atmosConfig, opts.Store)
	if err != nil {
		return err
	}

	// Open the planfile.
	f, err := os.Open(planfilePath)
	if err != nil {
		return fmt.Errorf("%w: failed to open planfile %s: %w", errUtils.ErrPlanfileUploadFailed, planfilePath, err)
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

	ui.Success(fmt.Sprintf("Uploaded planfile to %s: %s", store.Name(), key))
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

// resolveUploadPlanfilePath resolves the planfile path from the --planfile flag or derives it
// from --component and --stack using the Atmos stack configuration.
func resolveUploadPlanfilePath(opts *UploadOptions, atmosConfig *schema.AtmosConfiguration) (string, error) {
	defer perf.Track(atmosConfig, "planfile.resolveUploadPlanfilePath")()

	// If explicit --planfile flag was provided, use it directly.
	if opts.PlanfilePath != "" {
		return opts.PlanfilePath, nil
	}

	// Derive planfile path from component and stack.
	if opts.Component == "" || opts.Stack == "" {
		return "", fmt.Errorf("%w: --planfile is required when --component and --stack are not both provided", errUtils.ErrPlanfileUploadFailed)
	}

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: opts.Component,
		Stack:            opts.Stack,
		StackFromArg:     opts.Stack,
		ComponentType:    "terraform",
	}

	info, err := exec.ProcessStacks(atmosConfig, info, true, false, false, nil, nil)
	if err != nil {
		return "", fmt.Errorf("%w: failed to resolve component path: %w", errUtils.ErrPlanfileUploadFailed, err)
	}

	planfilePath := exec.ConstructTerraformComponentPlanfilePath(atmosConfig, &info)
	return planfilePath, nil
}

// getStoreOptions builds StoreOptions from atmos configuration.
// Precedence: explicit --store flag > S3 env vars > GitHub Actions env > local default.
func getStoreOptions(atmosConfig *schema.AtmosConfiguration, storeName string) (planfile.StoreOptions, error) {
	defer perf.Track(atmosConfig, "planfile.getStoreOptions")()

	// Explicit store name takes precedence.
	if storeName != "" {
		return planfile.StoreOptions{
			Type:        storeName,
			Options:     map[string]any{},
			AtmosConfig: atmosConfig,
		}, nil
	}

	// Try environment-based detection in order of precedence.
	if opts := detectS3FromEnv(); opts != nil {
		log.Debug("Storage provider: S3 from environment")
		opts.AtmosConfig = atmosConfig
		return *opts, nil
	}
	if opts := detectGitHubFromEnv(); opts != nil {
		log.Debug("Storage provider: GitHub from environment")
		opts.AtmosConfig = atmosConfig
		return *opts, nil
	}

	// Default to local storage.
	log.Debug("Storage provider: Local from environment")
	return defaultLocalStore(atmosConfig), nil
}

// detectS3FromEnv checks for S3 configuration in environment variables.
func detectS3FromEnv() *planfile.StoreOptions {
	bucket := os.Getenv("ATMOS_PLANFILE_BUCKET")
	if bucket == "" {
		return nil
	}
	return &planfile.StoreOptions{
		Type: "s3",
		Options: map[string]any{
			"bucket": bucket,
			"prefix": os.Getenv("ATMOS_PLANFILE_PREFIX"),
			"region": os.Getenv("AWS_REGION"),
		},
	}
}

// detectGitHubFromEnv checks if running in GitHub Actions.
func detectGitHubFromEnv() *planfile.StoreOptions {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return nil
	}
	return &planfile.StoreOptions{
		Type:    "github-artifacts",
		Options: map[string]any{},
	}
}

// defaultLocalStore returns the default local storage configuration.
func defaultLocalStore(atmosConfig *schema.AtmosConfiguration) planfile.StoreOptions {
	return planfile.StoreOptions{
		Type: "local",
		Options: map[string]any{
			"path": ".atmos/planfiles",
		},
		AtmosConfig: atmosConfig,
	}
}
