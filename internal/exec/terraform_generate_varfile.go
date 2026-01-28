package exec

import (
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provSource "github.com/cloudposse/atmos/pkg/provisioner/source"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ensureTerraformComponentExists checks if a terraform component exists and provisions it via JIT if needed.
// It returns an error if the component cannot be found or provisioned.
func ensureTerraformComponentExists(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	componentPath, err := u.GetComponentPath(atmosConfig, cfg.TerraformComponentType, info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return fmt.Errorf("failed to resolve component path: %w", err)
	}

	componentPathExists, _ := u.IsDirectory(componentPath)
	if componentPathExists {
		return nil
	}

	// Component doesn't exist - try JIT provisioning if source is configured.
	if err := tryJITProvision(atmosConfig, info); err != nil {
		return err
	}

	// Re-check if component exists after JIT provisioning.
	if _, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string); ok {
		return nil // Workdir path was set by provisioner.
	}

	componentPathExists, _ = u.IsDirectory(componentPath)
	if componentPathExists {
		return nil
	}

	// Component still doesn't exist.
	basePath, _ := u.GetComponentBasePath(atmosConfig, cfg.TerraformComponentType)
	return fmt.Errorf("%w: '%s' points to '%s', but it does not exist in '%s'",
		errUtils.ErrInvalidTerraformComponent, info.ComponentFromArg, info.FinalComponent, basePath)
}

// tryJITProvision attempts to provision a component via JIT if it has a source configured.
func tryJITProvision(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	if !provSource.HasSource(info.ComponentSection) {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := provSource.AutoProvisionSource(ctx, atmosConfig, cfg.TerraformComponentType, info.ComponentSection, info.AuthContext); err != nil {
		return fmt.Errorf("failed to auto-provision component source: %w", err)
	}

	return nil
}

// ExecuteGenerateVarfile generates a varfile for a terraform component.
func ExecuteGenerateVarfile(opts *VarfileOptions, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.ExecuteGenerateVarfile")()

	log.Debug("ExecuteGenerateVarfile called",
		"component", opts.Component,
		"stack", opts.Stack,
		"file", opts.File,
		"processTemplates", opts.ProcessTemplates,
		"processFunctions", opts.ProcessFunctions,
		"skip", opts.Skip,
	)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: opts.Component,
		Stack:            opts.Stack,
		StackFromArg:     opts.Stack,
		ComponentType:    cfg.TerraformComponentType,
		CliArgs:          []string{cfg.TerraformComponentType, "generate", "varfile"},
	}

	// Process stacks to get component configuration.
	info, err := ProcessStacks(atmosConfig, info, true, opts.ProcessTemplates, opts.ProcessFunctions, opts.Skip, nil)
	if err != nil {
		return err
	}

	// Ensure component exists, provisioning via JIT if needed.
	if err := ensureTerraformComponentExists(atmosConfig, &info); err != nil {
		return err
	}

	// Determine varfile path.
	var varFilePath string
	if len(opts.File) > 0 {
		varFilePath = opts.File
	} else {
		varFilePath = constructTerraformComponentVarfilePath(atmosConfig, &info)
	}

	// Print the component variables
	log.Debug("Generating varfile for variables",
		"component", info.ComponentFromArg,
		"stack", info.Stack,
		"variables", info.ComponentVarsSection,
	)

	// Write the variables to a file.
	log.Debug("Writing the variables to file", "file", varFilePath)

	if !info.DryRun {
		err = u.WriteToFileAsJSON(varFilePath, info.ComponentVarsSection, 0o644)
		if err != nil {
			return err
		}
	}

	return nil
}

// ExecuteTerraformGenerateVarfileCmd executes `terraform generate varfile` command.
// Deprecated: Use ExecuteGenerateVarfile with typed parameters instead.
func ExecuteTerraformGenerateVarfileCmd(cmd interface{}, args []string) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGenerateVarfileCmd")()

	return errUtils.ErrDeprecatedCmdNotCallable
}
