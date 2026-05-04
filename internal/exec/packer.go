package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	tfgenerate "github.com/cloudposse/atmos/pkg/terraform/generate"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const componentTypePacker = "packer"

// PackerFlags represents Packer command-line flags passed to ExecutePacker and ExecutePackerOutput.
type PackerFlags struct {
	// Template specifies the Packer template file or directory path.
	// If empty, defaults to "." (component working directory), which tells Packer to load all *.pkr.hcl files.
	// Can be set via --template/-t flag or settings.packer.template in stack manifest.
	Template string

	// Query specifies a YQ expression to extract data from the Packer manifest.
	// Used by ExecutePackerOutput to query the manifest.json file.
	// Can be set via --query/-q flag.
	Query string
}

// ExecutePacker executes Packer commands.
func ExecutePacker(
	info *schema.ConfigAndStacksInfo,
	packerFlags *PackerFlags,
) error {
	defer perf.Track(nil, "exec.ExecutePacker")()

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}

	// Validate packer configuration.
	if err := checkPackerConfig(&atmosConfig); err != nil {
		return err
	}

	// Add the `command` from `components.packer.command` from `atmos.yaml`.
	if info.Command == "" {
		if atmosConfig.Components.Packer.Command != "" {
			info.Command = atmosConfig.Components.Packer.Command
		} else {
			info.Command = cfg.PackerComponentType
		}
	}

	if info.SubCommand == "version" {
		tenv, err := dependencies.ForComponent(&atmosConfig, componentTypePacker, nil, nil)
		if err != nil {
			return err
		}
		return ExecuteShellCommand(
			atmosConfig,
			tenv.Resolve(info.Command),
			[]string{info.SubCommand},
			"",
			tenv.EnvVars(),
			false,
			info.RedirectStdErr,
		)
	}

	*info, err = ProcessStacks(&atmosConfig, *info, true, true, true, nil, nil)
	if err != nil {
		return err
	}

	if len(info.Stack) < 1 {
		return errUtils.ErrMissingStack
	}

	if !info.ComponentIsEnabled {
		log.Info("Component is not enabled and skipped", "component", info.ComponentFromArg)
		return nil
	}

	// Check if the component exists as a Packer component.
	componentPath, err := u.GetComponentPath(&atmosConfig, componentTypePacker, info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return fmt.Errorf("failed to resolve component path: %w", err)
	}

	// Auto-generate files BEFORE path validation when the following conditions hold.
	// 1. auto_generate_files is enabled.
	// 2. Component has a generate section.
	// 3. Not in dry-run mode (to avoid filesystem modifications).
	// This allows generating entire components from stack configuration.
	if atmosConfig.Components.Packer.AutoGenerateFiles && !info.DryRun { //nolint:nestif
		generateSection := tfgenerate.GetGenerateSectionFromComponent(info.ComponentSection)
		if generateSection != nil {
			// Ensure component directory exists for file generation.
			if mkdirErr := os.MkdirAll(componentPath, 0o755); mkdirErr != nil { //nolint:revive
				return errors.Join(errUtils.ErrCreateDirectory, fmt.Errorf("auto-generation: %w", mkdirErr))
			}

			// Generate files before path validation.
			if genErr := GenerateFilesForComponent(&atmosConfig, info, componentPath); genErr != nil {
				return errors.Join(errUtils.ErrFileOperation, genErr)
			}
		}
	}

	// Resolve the component path: existence check + JIT source provisioning +
	// metadata.component subpath join (issue #2364), all via the shared
	// orchestrator so packer honors metadata.component the same way terraform
	// does.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	componentPath, componentPathExists, err := component.ProvisionAndResolveComponentPath(
		ctx, &atmosConfig, info, cfg.PackerComponentType, componentPath,
	)
	if err != nil {
		return err
	}
	if !componentPathExists {
		basePath, _ := u.GetComponentBasePath(&atmosConfig, componentTypePacker)
		return fmt.Errorf(
			"%w: '%s' points to the Packer component '%s', but it does not exist in '%s'",
			errUtils.ErrInvalidComponent,
			info.ComponentFromArg,
			info.FinalComponent,
			basePath,
		)
	}

	// Check if the component is allowed to be provisioned (`metadata.type` attribute).
	// For Packer, only `build` creates external resources (AMIs, images, etc.).
	// Other commands (init, validate, inspect, fmt, console) are read-only or local operations.
	if info.ComponentIsAbstract && info.SubCommand == "build" {
		return fmt.Errorf("%w: the component '%s' cannot be provisioned because it's marked as abstract (metadata.type: abstract)",
			errUtils.ErrAbstractComponentCantBeProvisioned,
			filepath.Join(info.ComponentFolderPrefix, info.Component))
	}

	// Check if the component is locked (`metadata.locked` is set to true).
	if info.ComponentIsLocked && info.SubCommand == "build" {
		return fmt.Errorf("%w: component '%s' cannot be modified (metadata.locked: true)",
			errUtils.ErrLockedComponentCantBeProvisioned,
			filepath.Join(info.ComponentFolderPrefix, info.Component))
	}

	// Resolve and install component dependencies.
	tenv, err := dependencies.ForComponent(&atmosConfig, componentTypePacker, info.StackSection, info.ComponentSection)
	if err != nil {
		return err
	}
	info.ComponentEnvList = append(info.ComponentEnvList, tenv.EnvVars()...)

	// Check if the component 'settings.validation' section is specified and validate the component.
	valid, err := ValidateComponent(
		&atmosConfig,
		info.ComponentFromArg,
		info.ComponentSection,
		"",
		"",
		nil,
		0,
	)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf(
			"%w: the component '%s' did not pass the validation policies",
			errUtils.ErrInvalidComponent,
			info.ComponentFromArg,
		)
	}

	// Find Packer template.
	// It can be specified in the `settings.packer.template` section in the Atmos component manifest,
	// or on the command line via the flag `--template <template> (shorthand `-t`)`.
	template := packerFlags.Template
	if template == "" {
		packerSettingTemplate, err := GetPackerTemplateFromSettings(&info.ComponentSettingsSection)
		if err != nil {
			return err
		}

		if packerSettingTemplate != "" {
			template = packerSettingTemplate
		}
	}
	// If no template specified, default to "." (component working directory).
	// Packer will load all *.pkr.hcl files from the component directory.
	// This allows users to organize Packer configurations across multiple files.
	// For example: variables.pkr.hcl, main.pkr.hcl, locals.pkr.hcl.
	if template == "" {
		template = "."
	}

	// Print component variables.
	log.Debug("Variables for component in stack", "component", info.ComponentFromArg, "stack", info.Stack, "variables", info.ComponentVarsSection)

	// Write variables to a file.
	varFile := constructPackerComponentVarfileName(info)
	varFilePath := constructPackerComponentVarfilePath(&atmosConfig, info)

	log.Debug("Writing the variables to file", "file", varFilePath)

	if !info.DryRun {
		err = u.WriteToFileAsJSON(varFilePath, info.ComponentVarsSection, 0o644)
		if err != nil {
			return err
		}

		// Defer cleanup of the variable file.
		// Use a closure to capture varFilePath and ensure cleanup runs even on early errors.
		defer func() {
			if removeErr := os.Remove(varFilePath); removeErr != nil && !os.IsNotExist(removeErr) {
				log.Trace("Failed to remove var file during cleanup", "error", removeErr, "file", varFilePath)
			}
		}()
	}

	var inheritance string
	if len(info.ComponentInheritanceChain) > 0 {
		inheritance = info.ComponentFromArg + " -> " + strings.Join(info.ComponentInheritanceChain, " -> ")
	}

	workingDir := constructPackerComponentWorkingDir(&atmosConfig, info)

	log.Debug(
		"Packer context",
		"executable", info.Command,
		"command", info.SubCommand,
		"atmos component", info.ComponentFromArg,
		"atmos stack", info.StackFromArg,
		"packer component", info.BaseComponentPath,
		"packer template", template,
		"working directory", workingDir,
		"inheritance", inheritance,
		"arguments and flags", info.AdditionalArgsAndFlags,
	)

	// Prepare arguments and flags.
	allArgsAndFlags := []string{}
	allArgsAndFlags = append(allArgsAndFlags, info.SubCommand)
	allArgsAndFlags = append(allArgsAndFlags, []string{"-var-file", varFile}...)
	allArgsAndFlags = append(allArgsAndFlags, info.AdditionalArgsAndFlags...)
	allArgsAndFlags = append(allArgsAndFlags, template)

	// Convert ComponentEnvSection to ComponentEnvList.
	ConvertComponentEnvSectionToList(info)

	// Prepare ENV vars.
	envVars := append(info.ComponentEnvList, fmt.Sprintf("ATMOS_CLI_CONFIG_PATH=%s", atmosConfig.CliConfigPath))
	basePath, err := filepath.Abs(atmosConfig.BasePath)
	if err != nil {
		return err
	}
	envVars = append(envVars, fmt.Sprintf("ATMOS_BASE_PATH=%s", basePath))
	log.Debug("Using ENV", "variables", envVars)

	return ExecuteShellCommand(
		atmosConfig,
		info.Command,
		allArgsAndFlags,
		componentPath,
		envVars,
		info.DryRun,
		info.RedirectStdErr,
		WithEnvironment(info.SanitizedEnv),
	)
}
