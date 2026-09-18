package planfile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	"github.com/cloudposse/atmos/pkg/ci/providers/generic"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/version"
)

// uploadParser handles flag parsing with Viper precedence for the upload command.
var uploadParser *flags.StandardParser

// UploadOptions contains parsed flags for the upload command.
type UploadOptions struct {
	BaseOptions
	PlanfilePath string
	LockfilePath string
	Component    string
	SHA          string
}

var uploadCmd = &cobra.Command{
	Use:   "upload <component>",
	Short: "Upload a Terraform plan file to storage",
	Long: `Upload a Terraform plan file to the configured storage backend.

The component is specified as a positional argument and the stack via -s/--stack.
The storage backend is configured in atmos.yaml under terraform.planfiles.
Supported backends: local/dir, aws/s3, github/artifacts.

When --planfile is omitted, the planfile path is derived from component and stack.`,
	Args: cobra.ExactArgs(1),
	RunE: runUpload,
}

func init() {
	// Create parser with upload-specific flags using functional options.
	uploadParser = flags.NewStandardParser(
		flags.WithStringFlag("store", "", "", "Storage backend to use (default from config)"),
		flags.WithStringFlag("planfile", "", "", "Path to the planfile to upload (default: derived from component/stack)"),
		flags.WithStringFlag("sha", "", "", "Git SHA for metadata (default: current HEAD)"),
		flags.WithStringFlag("lockfile", "", "", "Path to .terraform.lock.hcl (default: auto-detected from planfile path)"),
		flags.WithEnvVars("store", "ATMOS_PLANFILE_STORE"),
		flags.WithEnvVars("planfile", "ATMOS_PLANFILE_PATH"),
		flags.WithEnvVars("sha", "ATMOS_PLANFILE_SHA"),
		flags.WithEnvVars("lockfile", "ATMOS_PLANFILE_LOCKFILE"),
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
		PlanfilePath: v.GetString("planfile"),
		LockfilePath: v.GetString("lockfile"),
		Component:    args[0],
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

	// Bind persistent parent flags too.
	if err := planfileParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Parse options.
	opts := parseUploadOptions(cmd, v, args)

	// Validate that stack is provided.
	if opts.Stack == "" {
		return fmt.Errorf("%w: --stack/-s is required for upload", errUtils.ErrPlanfileStoreInvalidArgs)
	}

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
	planFile, err := os.Open(planfilePath)
	if err != nil {
		return fmt.Errorf("%w: failed to open planfile %s: %w", errUtils.ErrPlanfileUploadFailed, planfilePath, err)
	}
	defer planFile.Close()

	// Build file entries for upload.
	files := []planfile.FileEntry{
		{Name: planfile.PlanFilename, Data: planFile, Size: -1},
	}

	// Resolve lock file path.
	lockfilePath := resolveLockfilePath(opts.LockfilePath, planfilePath)
	if lockfilePath != "" {
		if lf, err := os.Open(lockfilePath); err == nil {
			defer lf.Close()
			files = append(files, planfile.FileEntry{Name: planfile.LockFilename, Data: lf, Size: -1})
		}
	}

	// Resolve SHA: explicit flag > context resolution.
	sha := opts.SHA
	if sha == "" {
		resolved, err := resolveContext(false)
		if err != nil {
			return err
		}
		sha = resolved.SHA
	}

	// Build metadata and generate key.
	metadata := buildUploadMetadata(opts, sha)
	key, err := resolveKey(opts.Component, opts.Stack, sha)
	if err != nil {
		return err
	}

	// Upload the files.
	ctx := context.Background()
	if err := store.Upload(ctx, key, files, metadata); err != nil {
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

// buildUploadMetadata creates metadata for the planfile upload.
// Populates CI context fields (Branch, RunID, Repository, etc.) from
// environment variables via the generic CI provider.
func buildUploadMetadata(opts *UploadOptions, sha string) *planfile.Metadata {
	defer perf.Track(nil, "planfile.buildUploadMetadata")()

	metadata := &planfile.Metadata{}
	metadata.Stack = opts.Stack
	metadata.Component = opts.Component
	metadata.SHA = sha
	metadata.CreatedAt = time.Now()
	metadata.AtmosVersion = version.Version

	// Populate CI context fields from environment.
	ciProvider := generic.NewProvider()
	ciCtx, err := ciProvider.Context()
	if err == nil && ciCtx != nil {
		// Use CI context SHA if not explicitly provided.
		if metadata.SHA == "" {
			metadata.SHA = ciCtx.SHA
		}
		metadata.Branch = ciCtx.Branch
		metadata.RunID = ciCtx.RunID
		metadata.Repository = ciCtx.Repository
	}

	return metadata
}

// resolveUploadPlanfilePath resolves the planfile path from the --planfile flag or derives it
// from component and stack using the Atmos stack configuration.
func resolveUploadPlanfilePath(opts *UploadOptions, atmosConfig *schema.AtmosConfiguration) (string, error) {
	defer perf.Track(atmosConfig, "planfile.resolveUploadPlanfilePath")()

	// If explicit --planfile flag was provided, use it directly.
	if opts.PlanfilePath != "" {
		return opts.PlanfilePath, nil
	}

	// Derive planfile path from component and stack.
	if opts.Component == "" || opts.Stack == "" {
		return "", fmt.Errorf("%w: --planfile is required when component and stack are not both provided", errUtils.ErrPlanfileUploadFailed)
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

// resolveLockfilePath returns the lock file path from the explicit flag or auto-detected
// from the planfile's directory.
func resolveLockfilePath(explicit, planfilePath string) string {
	if explicit != "" {
		return explicit
	}
	// Auto-detect: look for .terraform.lock.hcl in the same directory as the planfile.
	candidate := filepath.Join(filepath.Dir(planfilePath), planfile.LockFilename)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}
