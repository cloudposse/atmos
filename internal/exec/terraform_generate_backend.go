package exec

import (
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// validateBackendConfig validates the backend configuration for the component.
func validateBackendConfig(info *schema.ConfigAndStacksInfo) error {
	if info.ComponentBackendType == "" {
		return errUtils.ErrMissingTerraformBackendType
	}
	if info.ComponentBackendSection == nil {
		return errUtils.ErrMissingTerraformBackendConfig
	}
	return nil
}

// validateBackendTypeRequirements validates backend-type-specific requirements.
func validateBackendTypeRequirements(info *schema.ConfigAndStacksInfo) error {
	switch info.ComponentBackendType {
	case cfg.BackendTypeS3:
		if _, ok := info.ComponentBackendSection["workspace_key_prefix"].(string); !ok {
			return errUtils.ErrMissingTerraformWorkspaceKeyPrefix
		}
	case cfg.BackendTypeGCS:
		if _, ok := info.ComponentBackendSection["bucket"].(string); !ok {
			return errUtils.ErrGCSBucketRequired
		}
	}
	return nil
}

// ExecuteGenerateBackend generates backend config for a terraform component.
func ExecuteGenerateBackend(opts *GenerateBackendOptions, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.ExecuteGenerateBackend")()

	log.Debug("ExecuteGenerateBackend called",
		"component", opts.Component,
		"stack", opts.Stack,
		"processTemplates", opts.ProcessTemplates,
		"processFunctions", opts.ProcessFunctions,
		"skip", opts.Skip,
	)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: opts.Component,
		Stack:            opts.Stack,
		StackFromArg:     opts.Stack,
		ComponentType:    "terraform",
		CliArgs:          []string{"terraform", "generate", "backend"},
	}

	// Process stacks to get component configuration.
	info, err := ProcessStacks(atmosConfig, info, true, opts.ProcessTemplates, opts.ProcessFunctions, opts.Skip, nil)
	if err != nil {
		return err
	}

	if err := validateBackendConfig(&info); err != nil {
		return err
	}

	componentBackendConfig, err := generateComponentBackendConfig(info.ComponentBackendType, info.ComponentBackendSection, info.TerraformWorkspace, info.AuthContext)
	if err != nil {
		return err
	}

	log.Debug("Component backend", "config", componentBackendConfig)

	if err := validateBackendTypeRequirements(&info); err != nil {
		return err
	}

	return writeBackendConfigFile(atmosConfig, &info, componentBackendConfig)
}

// writeBackendConfigFile writes the backend config to a file.
func writeBackendConfigFile(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, config map[string]any) error {
	backendFilePath := filepath.Join(
		atmosConfig.BasePath,
		atmosConfig.Components.Terraform.BasePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
		"backend.tf.json",
	)

	log.Debug("Writing the backend config to file", "file", backendFilePath)

	if info.DryRun {
		return nil
	}
	return u.WriteToFileAsJSON(backendFilePath, config, filePermissions)
}

// ExecuteTerraformGenerateBackendCmd executes `terraform generate backend` command.
// Deprecated: Use ExecuteGenerateBackend with typed parameters instead.
func ExecuteTerraformGenerateBackendCmd(cmd interface{}, args []string) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGenerateBackendCmd")()

	return errUtils.ErrDeprecatedCmdNotCallable
}
