package exec

import (
	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

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
		ComponentType:    "terraform",
		CliArgs:          []string{"terraform", "generate", "varfile"},
	}

	// Process stacks to get component configuration.
	info, err := ProcessStacks(atmosConfig, info, true, opts.ProcessTemplates, opts.ProcessFunctions, opts.Skip, nil)
	if err != nil {
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
