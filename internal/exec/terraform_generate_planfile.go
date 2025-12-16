package exec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	tf "github.com/cloudposse/atmos/pkg/terraform"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	ErrInvalidFormat                      = errors.New("invalid format")
	ErrCreatingTempDirectory              = errors.New("error creating temporary directory")
	ErrCreatingIntermediateSubdirectories = errors.New("error creating intermediate subdirectories")
	ErrGettingJsonForPlanfile             = errors.New("error getting JSON for planfile")
	ErrConvertingJsonToGoType             = errors.New("error converting JSON to Go type")
	ErrNoComponent                        = errors.New("no component specified")
)

// PlanfileOptions is an alias to pkg/terraform.PlanfileOptions for backwards compatibility.
type PlanfileOptions = tf.PlanfileOptions

// ExecuteGeneratePlanfile generates a planfile for a terraform component.
func ExecuteGeneratePlanfile(opts *PlanfileOptions, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.ExecuteGeneratePlanfile")()

	log.Debug("ExecuteGeneratePlanfile called",
		"component", opts.Component,
		"stack", opts.Stack,
		"file", opts.File,
		"outputPath", opts.OutputPath,
		"format", opts.Format,
		"processTemplates", opts.ProcessTemplates,
		"processFunctions", opts.ProcessYamlFunctions,
		"skip", opts.Skip,
	)

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: opts.Component,
		Stack:            opts.Stack,
		StackFromArg:     opts.Stack,
		ComponentType:    "terraform",
		CliArgs:          []string{"terraform", "generate", "planfile"},
	}

	return ExecuteTerraformGeneratePlanfile(opts, info)
}

// ExecuteTerraformGeneratePlanfileCmd executes `terraform generate planfile` command.
//
// Deprecated: Use ExecuteGeneratePlanfile with typed parameters instead.
// This function will be removed in a future release.
func ExecuteTerraformGeneratePlanfileCmd(_ interface{}, _ []string) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGeneratePlanfileCmd")()

	return errUtils.ErrDeprecatedCmdNotCallable
}

// ExecuteTerraformGeneratePlanfileOld executes `terraform generate planfile` command.
func ExecuteTerraformGeneratePlanfileOld(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGeneratePlanfileCmd")()

	if len(args) == 0 {
		return ErrNoComponent
	}

	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
	if err != nil {
		return err
	}

	file, err := flags.GetString("file")
	if err != nil {
		return err
	}

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}

	processTemplates, err := flags.GetBool("process-templates")
	if err != nil {
		return err
	}

	processYamlFunctions, err := flags.GetBool("process-functions")
	if err != nil {
		return err
	}

	skip, err := flags.GetStringSlice("skip")
	if err != nil {
		return err
	}

	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	info.CliArgs = []string{"terraform", "generate", "planfile"}

	component := args[0]

	options := PlanfileOptions{
		Component:            component,
		Stack:                stack,
		Format:               format,
		File:                 file,
		ProcessTemplates:     processTemplates,
		ProcessYamlFunctions: processYamlFunctions,
		Skip:                 skip,
	}

	return ExecuteTerraformGeneratePlanfile(&options, &info)
}

// ExecuteTerraformGeneratePlanfile executes `terraform generate planfile`.
func ExecuteTerraformGeneratePlanfile(
	options *PlanfileOptions,
	info *schema.ConfigAndStacksInfo,
) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGeneratePlanfile")()

	if err := validatePlanfileFormat(&options.Format); err != nil {
		return err
	}

	if err := validateComponent(options.Component); err != nil {
		return err
	}

	info.ComponentFromArg = options.Component
	info.Stack = options.Stack
	info.ComponentType = "terraform"
	info.NeedHelp = false

	// Process templates and Atmos YAML functions.
	info.ProcessTemplates = options.ProcessTemplates
	info.ProcessFunctions = options.ProcessYamlFunctions

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}

	*info, err = ProcessStacks(&atmosConfig, *info, true, options.ProcessTemplates, options.ProcessYamlFunctions, options.Skip, nil)
	if err != nil {
		return err
	}

	componentPath := filepath.Join(atmosConfig.TerraformDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)

	// Create a temporary directory for all temporary files.
	tmpDir, err := os.MkdirTemp("", "atmos-terraform-generate-planfile")
	if err != nil {
		return errors.Join(ErrCreatingTempDirectory, err)
	}

	defer func(path string) {
		err = os.RemoveAll(path)
		if err != nil {
			log.Warn("Error removing temporary directory", "path", path, "error", err)
		}
	}(tmpDir)

	// Generate planfile in the temp directory.
	planFile, err := generateNewPlanFile(&atmosConfig, info, componentPath, tmpDir)
	if err != nil {
		return err
	}

	// Get the JSON representation of the new plan.
	planJSON, err := getTerraformPlanJSON(&atmosConfig, info, componentPath, planFile)
	if err != nil {
		return errors.Join(ErrGettingJsonForPlanfile, err)
	}

	// Resolve the planfile path based on options. If a custom file is specified, use that. Otherwise, use the default path.
	pathOpts := planfilePathOptions{
		componentPath: componentPath,
		format:        options.Format,
		customFile:    options.File,
		outputPath:    options.OutputPath,
	}
	planFilePath, err := resolvePlanfilePath(pathOpts, info, &atmosConfig)
	if err != nil {
		return err
	}

	log.Debug("Writing the planfile", "file", planFilePath)

	// Write the planfile in JSON or YAML format.
	err = writePlanfile(planFilePath, options.Format, planJSON)
	if err != nil {
		return err
	}

	return nil
}

// validatePlanfileFormat checks if the format is valid and sets default if empty.
func validatePlanfileFormat(format *string) error {
	if *format == "" {
		*format = "json"
	}

	if *format != "json" && *format != "yaml" {
		return fmt.Errorf("%w: %s. Supported formats are 'json' and 'yaml'", ErrInvalidFormat, *format)
	}
	return nil
}

// validateComponent checks if the provided component is not empty.
func validateComponent(component string) error {
	if component == "" {
		return ErrNoComponent
	}
	return nil
}

// planfilePathOptions holds parameters for resolving planfile path.
type planfilePathOptions struct {
	componentPath string
	format        string
	customFile    string
	outputPath    string
}

// resolvePlanfilePath determines the final path for the planfile based on options.
// It handles three modes: CustomFile set uses exact path (absolute or relative to component).
// OutputPath set uses default filename in specified directory. Neither uses atmos config default path.
func resolvePlanfilePath(opts planfilePathOptions, info *schema.ConfigAndStacksInfo, atmosConfig *schema.AtmosConfiguration) (string, error) {
	var planFilePath string

	switch {
	case opts.customFile != "":
		// Mode 1: Exact file path specified.
		if filepath.IsAbs(opts.customFile) {
			planFilePath = opts.customFile
		} else {
			planFilePath = filepath.Join(opts.componentPath, opts.customFile)
		}
	case opts.outputPath != "":
		// Mode 2: Output directory specified, use default filename.
		defaultFilename := fmt.Sprintf("%s-%s.planfile.%s", info.StackFromArg, info.ComponentFromArg, opts.format)
		if filepath.IsAbs(opts.outputPath) {
			planFilePath = filepath.Join(opts.outputPath, defaultFilename)
		} else {
			planFilePath = filepath.Join(opts.componentPath, opts.outputPath, defaultFilename)
		}
	default:
		// Mode 3: Use atmos config default path.
		planFilePath = fmt.Sprintf("%s.%s", constructTerraformComponentPlanfilePath(atmosConfig, info), opts.format)
	}

	err := u.EnsureDir(planFilePath)
	if err != nil {
		return "", errors.Join(ErrCreatingIntermediateSubdirectories, err)
	}

	return planFilePath, nil
}

// writePlanfile writes the planfile in the specified format.
func writePlanfile(planFilePath, format string, planJSON string) error {
	d, err := u.ConvertFromJSON(planJSON)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrConvertingJsonToGoType, err)
	}

	const fileMode = 0o644
	if format == "json" {
		err = u.WriteToFileAsJSON(planFilePath, d, fileMode)
	} else {
		err = u.WriteToFileAsYAML(planFilePath, d, fileMode)
	}

	return err
}
