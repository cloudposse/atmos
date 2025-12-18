package planfile

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/planfile"

	// Import planfile store implementations to register them.
	_ "github.com/cloudposse/atmos/pkg/ci/planfile/github"
	_ "github.com/cloudposse/atmos/pkg/ci/planfile/local"
	_ "github.com/cloudposse/atmos/pkg/ci/planfile/s3"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var uploadCmd = &cobra.Command{
	Use:   "upload <planfile>",
	Short: "Upload a Terraform plan file to storage",
	Long: `Upload a Terraform plan file to the configured storage backend.

The storage backend is configured in atmos.yaml under terraform.planfiles.
Supported backends: local, s3, github-artifacts.`,
	Args: cobra.ExactArgs(1),
	RunE: runUpload,
}

var (
	uploadStore     string
	uploadKey       string
	uploadStack     string
	uploadComponent string
	uploadSHA       string
)

func init() {
	uploadCmd.Flags().StringVar(&uploadStore, "store", "", "Storage backend to use (default from config)")
	uploadCmd.Flags().StringVar(&uploadKey, "key", "", "Storage key (default: generated from stack/component/SHA)")
	uploadCmd.Flags().StringVar(&uploadStack, "stack", "", "Stack name for metadata")
	uploadCmd.Flags().StringVar(&uploadComponent, "component", "", "Component name for metadata")
	uploadCmd.Flags().StringVar(&uploadSHA, "sha", "", "Git SHA for metadata (default: current HEAD)")
}

func runUpload(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "planfile.runUpload")()

	planfilePath := args[0]

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return err
	}

	// Get the storage configuration.
	storeOpts, err := getStoreOptions(&atmosConfig, uploadStore)
	if err != nil {
		return err
	}

	// Create the store.
	store, err := planfile.NewStore(storeOpts)
	if err != nil {
		return err
	}

	// Open the planfile.
	f, err := os.Open(planfilePath)
	if err != nil {
		return fmt.Errorf("%w: failed to open planfile %s: %w", errUtils.ErrPlanfileUploadFailed, planfilePath, err)
	}
	defer f.Close()

	// Build metadata.
	metadata := &planfile.Metadata{
		Stack:     uploadStack,
		Component: uploadComponent,
		SHA:       uploadSHA,
		CreatedAt: time.Now(),
	}

	// Generate key if not provided.
	key := uploadKey
	if key == "" {
		keyPattern := planfile.DefaultKeyPattern()
		key, err = keyPattern.GenerateKey(&planfile.KeyContext{
			Stack:     uploadStack,
			Component: uploadComponent,
			SHA:       uploadSHA,
		})
		if err != nil {
			return fmt.Errorf("failed to generate planfile key: %w", err)
		}
	}

	// Upload.
	ctx := context.Background()
	if err := store.Upload(ctx, key, f, metadata); err != nil {
		return err
	}

	_ = ui.Success(fmt.Sprintf("Uploaded planfile to %s: %s", store.Name(), key))
	return nil
}

// getStoreOptions builds StoreOptions from atmos configuration.
func getStoreOptions(atmosConfig *schema.AtmosConfiguration, storeName string) (planfile.StoreOptions, error) {
	defer perf.Track(atmosConfig, "planfile.getStoreOptions")()

	// For now, use defaults. In a full implementation, this would read from
	// atmosConfig.Terraform.Planfiles configuration.
	storeType := "local"
	options := map[string]any{
		"path": ".atmos/planfiles",
	}

	if storeName != "" {
		storeType = storeName
	}

	// Check environment for S3 configuration.
	if bucket := os.Getenv("ATMOS_PLANFILE_BUCKET"); bucket != "" {
		storeType = "s3"
		options = map[string]any{
			"bucket": bucket,
			"prefix": os.Getenv("ATMOS_PLANFILE_PREFIX"),
			"region": os.Getenv("AWS_REGION"),
		}
	}

	// Check environment for GitHub configuration.
	if os.Getenv("GITHUB_ACTIONS") == "true" && storeType == "" {
		storeType = "github-artifacts"
		options = map[string]any{}
	}

	return planfile.StoreOptions{
		Type:        storeType,
		Options:     options,
		AtmosConfig: atmosConfig,
	}, nil
}
