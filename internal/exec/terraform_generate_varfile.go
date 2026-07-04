package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ensureTerraformComponentExists checks if a terraform component exists and
// provisions it via JIT when source.uri is configured. Errors are wrapped
// with ErrInvalidTerraformComponent.
func ensureTerraformComponentExists(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	componentPath, err := u.GetComponentPath(atmosConfig, cfg.TerraformComponentType, info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return errors.Join(errUtils.ErrInvalidTerraformComponent, fmt.Errorf("failed to resolve component path: %w", err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	_, exists, err := component.ProvisionAndResolveComponentPath(
		ctx, atmosConfig, info, cfg.TerraformComponentType, componentPath,
	)
	if err != nil {
		return errors.Join(errUtils.ErrInvalidTerraformComponent, err)
	}
	if exists {
		return nil
	}

	// WorkdirPathKey may have been pre-populated by an upstream provisioner
	// (the orchestrator only sets it for components declaring source.uri).
	// Trust it as authoritative when present.
	if workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string); ok && workdirPath != "" {
		return nil
	}

	basePath, err := u.GetComponentBasePath(atmosConfig, cfg.TerraformComponentType)
	if err != nil {
		return errors.Join(errUtils.ErrInvalidTerraformComponent, fmt.Errorf("failed to resolve component base path: %w", err))
	}
	return fmt.Errorf("%w: '%s' points to '%s', but it does not exist in '%s'",
		errUtils.ErrInvalidTerraformComponent, info.ComponentFromArg, info.FinalComponent, basePath)
}

// ExecuteGenerateVarfile generates a varfile for a terraform component.
func ExecuteGenerateVarfile(opts *VarfileOptions, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.ExecuteGenerateVarfile")()

	log.Debug(
		"ExecuteGenerateVarfile called",
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
	log.Debug(
		"Generating varfile for variables",
		"component", info.ComponentFromArg,
		"stack", info.Stack,
		"variables", info.ComponentVarsSection,
	)

	// Display the varfile path relative to the current working directory.
	displayPath := relativeToCwd(varFilePath)

	// Write the variables to a file.
	log.Debug("Writing the variables to file", "file", displayPath)

	varsToWrite := varfileVarsToWrite(&info, opts.WithSecrets, displayPath)

	if !info.DryRun {
		err = u.WriteToFileAsJSON(varFilePath, varsToWrite, filePermissions)
		if err != nil {
			return err
		}
		if opts.WithSecrets {
			ui.Successf("Generated varfile `%s` (with secrets)", displayPath)
		} else {
			ui.Successf("Generated varfile `%s`", displayPath)
		}
	}

	return nil
}

// relativeToCwd returns p relative to the current working directory for display. It falls
// back to the original path when the working directory can't be determined or p can't be
// made relative (e.g. on a different volume).
func relativeToCwd(p string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return p
	}
	rel, err := filepath.Rel(cwd, p)
	if err != nil {
		return p
	}
	return rel
}

// varfileVarsToWrite returns the variables to write to a generated varfile. By default,
// secret-bearing variables are omitted so plaintext secrets never hit disk; with
// withSecrets=true they are included (e.g. to export the file). An appropriate warning is
// emitted in each case. Requires that secrets have already been resolved into info.
func varfileVarsToWrite(info *schema.ConfigAndStacksInfo, withSecrets bool, varFilePath string) map[string]any {
	computeTerraformSecretVarKeys(info)

	if withSecrets {
		if len(info.TerraformSecretVarKeys) > 0 {
			log.Debug("Writing resolved secret values to the varfile in plaintext (--with-secrets)",
				"file", varFilePath, "count", len(info.TerraformSecretVarKeys))
		}
		return info.ComponentVarsSection
	}

	if len(info.TerraformSecretVarKeys) > 0 {
		log.Warn("Omitting secrets from the generated varfile; pass --with-secrets to include them",
			"file", varFilePath, "count", len(info.TerraformSecretVarKeys))
	}
	return diskSafeVars(info)
}

// ExecuteTerraformGenerateVarfileCmd executes `terraform generate varfile` command.
// Deprecated: Use ExecuteGenerateVarfile with typed parameters instead.
func ExecuteTerraformGenerateVarfileCmd(cmd interface{}, args []string) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGenerateVarfileCmd")()

	return errUtils.ErrDeprecatedCmdNotCallable
}
