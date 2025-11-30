package exec

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteGenerateBackend generates backend config for a terraform component.
func ExecuteGenerateBackend(
	component, stack string,
	processTemplates, processFunctions bool,
	skip []string,
	atmosConfig *schema.AtmosConfiguration,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteGenerateBackend")()

	log.Debug("ExecuteGenerateBackend called",
		"component", component,
		"stack", stack,
		"processTemplates", processTemplates,
		"processFunctions", processFunctions,
		"skip", skip,
	)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: component,
		Stack:            stack,
		StackFromArg:     stack,
		ComponentType:    "terraform",
		CliArgs:          []string{"terraform", "generate", "backend"},
	}

	// Process stacks to get component configuration.
	info, err := ProcessStacks(atmosConfig, info, true, processTemplates, processFunctions, skip, nil)
	if err != nil {
		return err
	}

	if info.ComponentBackendType == "" {
		return fmt.Errorf("'backend_type' is missing for the '%s' component", component)
	}

	if info.ComponentBackendSection == nil {
		return fmt.Errorf("could not find 'backend' config for the '%s' component", component)
	}

	componentBackendConfig, err := generateComponentBackendConfig(info.ComponentBackendType, info.ComponentBackendSection, info.TerraformWorkspace, info.AuthContext)
	if err != nil {
		return err
	}

	log.Debug("Component backend", "config", componentBackendConfig)

	// Check if the `backend` section has `workspace_key_prefix` when `backend_type` is `s3`
	if info.ComponentBackendType == cfg.BackendTypeS3 {
		if _, ok := info.ComponentBackendSection["workspace_key_prefix"].(string); !ok {
			return fmt.Errorf("backend config for the '%s' component is missing 'workspace_key_prefix'", component)
		}
	}

	// Check if the `backend` section has `bucket` when `backend_type` is `gcs`
	if info.ComponentBackendType == cfg.BackendTypeGCS {
		if _, ok := info.ComponentBackendSection["bucket"].(string); !ok {
			return errUtils.ErrGCSBucketRequired
		}
	}

	// Write the backend config to a file
	backendFilePath := filepath.Join(
		atmosConfig.BasePath,
		atmosConfig.Components.Terraform.BasePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
		"backend.tf.json",
	)

	log.Debug("Writing the backend config to file", "file", backendFilePath)

	if !info.DryRun {
		err = u.WriteToFileAsJSON(backendFilePath, componentBackendConfig, 0o644)
		if err != nil {
			return err
		}
	}

	return nil
}

// ExecuteTerraformGenerateBackendCmd executes `terraform generate backend` command.
// Deprecated: Use ExecuteGenerateBackend with typed parameters instead.
func ExecuteTerraformGenerateBackendCmd(cmd interface{}, args []string) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGenerateBackendCmd")()

	return errors.New("ExecuteTerraformGenerateBackendCmd is deprecated and should not be called")
}
