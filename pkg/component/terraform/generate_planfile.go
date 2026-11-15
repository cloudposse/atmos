package terraform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/google/shlex"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	envpkg "github.com/cloudposse/atmos/pkg/env"
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
func (s *Operations) ExecuteGeneratePlanfile(opts *PlanfileOptions, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.ExecuteGeneratePlanfile")()

	log.Debug(
		"ExecuteGeneratePlanfile called",
		"component", opts.Component,
		"stack", opts.Stack,
		"file", opts.File,
		"format", opts.Format,
		"processTemplates", opts.ProcessTemplates,
		"processFunctions", opts.ProcessYamlFunctions,
		"skip", opts.Skip,
	)

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: opts.Component,
		Stack:            opts.Stack,
		StackFromArg:     opts.Stack,
		ComponentType:    cfg.TerraformComponentType,
		CliArgs:          []string{cfg.TerraformComponentType, "generate", "planfile"},
	}

	return s.ExecuteTerraformGeneratePlanfile(opts, info)
}

// ExecuteTerraformGeneratePlanfileOld executes `terraform generate planfile` command.
func (s *Operations) ExecuteTerraformGeneratePlanfileOld(cmd *cobra.Command, args []string) error {
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

	info, err := s.host.ProcessCommandLineArgs(cfg.TerraformComponentType, cmd, args, nil)
	if err != nil {
		return err
	}

	info.CliArgs = []string{cfg.TerraformComponentType, "generate", "planfile"}

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

	return s.ExecuteTerraformGeneratePlanfile(&options, &info)
}

// ExecuteTerraformGeneratePlanfile executes `terraform generate planfile`.
//
//nolint:revive,funlen // Preserve the existing generation and evaluation order during package extraction.
func (s *Operations) ExecuteTerraformGeneratePlanfile(
	options *PlanfileOptions,
	info *schema.ConfigAndStacksInfo,
) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGeneratePlanfile")()

	if err := ValidatePlanfileFormat(&options.Format); err != nil {
		return err
	}

	if err := ValidateComponent(options.Component); err != nil {
		return err
	}

	info.ComponentFromArg = options.Component
	info.Stack = options.Stack
	info.ComponentType = cfg.TerraformComponentType
	info.NeedHelp = false

	// Process templates and Atmos YAML functions.
	info.ProcessTemplates = options.ProcessTemplates
	info.ProcessFunctions = options.ProcessYamlFunctions

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}

	*info, err = s.host.ProcessStacks(&atmosConfig, *info, true, options.ProcessTemplates, options.ProcessYamlFunctions, options.Skip, nil)
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
	planFile, err := s.GenerateNewPlanFile(&atmosConfig, info, componentPath, tmpDir)
	if err != nil {
		return err
	}

	// Get the JSON representation of the new plan.
	planJSON, err := s.getTerraformPlanJSON(&atmosConfig, info, componentPath, planFile)
	if err != nil {
		return errors.Join(ErrGettingJsonForPlanfile, err)
	}

	// Resolve the planfile path based on options. If a custom file is specified, use that. Otherwise, use the default path.
	planFilePath, err := resolvePlanfilePath(componentPath, options.Format, options.File, info, &atmosConfig)
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

// ValidatePlanfileFormat checks if the format is valid and sets default if empty.
func ValidatePlanfileFormat(format *string) error {
	defer perf.Track(nil, "terraform.ValidatePlanfileFormat")()

	if *format == "" {
		*format = "json"
	}

	if *format != "json" && *format != "yaml" {
		return fmt.Errorf("%w: %s. Supported formats are 'json' and 'yaml'", ErrInvalidFormat, *format)
	}
	return nil
}

// ValidateComponent checks if the provided component is not empty.
func ValidateComponent(component string) error {
	defer perf.Track(nil, "terraform.ValidateComponent")()

	if component == "" {
		return ErrNoComponent
	}
	return nil
}

// resolvePlanfilePath determines the final path for the planfile based on options.
func resolvePlanfilePath(componentPath, format string, customFile string, info *schema.ConfigAndStacksInfo, atmosConfig *schema.AtmosConfiguration) (string, error) {
	var planFilePath string
	if customFile != "" {
		if filepath.IsAbs(customFile) {
			planFilePath = customFile
		} else {
			planFilePath = filepath.Join(componentPath, customFile)
		}
	} else {
		planFilePath = fmt.Sprintf("%s.%s", ConstructTerraformComponentPlanfilePath(atmosConfig, info), format)
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
		return fmt.Errorf("%w: %w", ErrConvertingJsonToGoType, err)
	}

	const fileMode = 0o644
	if format == "json" {
		err = u.WriteToFileAsJSON(planFilePath, d, fileMode)
	} else {
		err = u.WriteToFileAsYAML(planFilePath, d, fileMode)
	}

	return err
}

// validatePlanOutputFlags rejects ambiguous plan destinations before hooks or Terraform run.
func validatePlanOutputFlags(command string, args []string) error {
	if command != "plan" {
		return nil
	}
	count := 0
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-out" || arg == "--out" {
			count++
			i++
		} else if strings.HasPrefix(arg, "-out=") || strings.HasPrefix(arg, "--out=") {
			count++
		}
		if count > 1 {
			return fmt.Errorf("%w: multiple plan output flags", errUtils.ErrPlanVerificationFailed)
		}
	}
	return nil
}

// validateEnvironmentPlanOutput rejects plan output flags in TF_CLI_ARGS or
// TF_CLI_ARGS_plan for a configuration-owned plan. Terraform reads these
// variables itself, so a flag there could choose a plan file that Atmos does not
// see and therefore does not bind. The check inspects the effective value of each
// variable in the subprocess environment: the sanitized process environment (or
// the inherited one when auth did not sanitize it), the global atmos.yaml
// environment, the component environment, and provider extra-argument
// environment, with later layers overriding earlier ones. Hooks run as child
// processes and cannot change it.
func validateEnvironmentPlanOutput(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, execCtx *componentExecContext) error {
	if execCtx.preparedConfiguration == nil || info.DryRun || info.SubCommand != subcommandPlan {
		return nil
	}
	return rejectEnvironmentPlanOutput(planEnvironment(atmosConfig, info), runtime.GOOS)
}

// rejectEnvironmentPlanOutput rejects a plan output flag in the effective TF_CLI_ARGS or
// TF_CLI_ARGS_plan of an environment listed in precedence order. Variable names are
// case-insensitive on Windows only, where the last matching entry is the effective value.
func rejectEnvironmentPlanOutput(entries []string, goos string) error {
	for _, name := range []string{tfCliArgsEnvVar, tfCliArgsEnvVar + "_" + subcommandPlan} {
		value, _ := lastEnvListValue(entries, name, goos)
		args, err := tokenizeEnvironmentArgs(value)
		if err != nil {
			return fmt.Errorf("%w: %s cannot be parsed: %w", errUtils.ErrPlanVerificationFailed, name, err)
		}
		if containsOutputFlag(args) {
			return fmt.Errorf("%w: %s sets a plan output flag; pass -out as a command argument instead", errUtils.ErrPlanVerificationFailed, name)
		}
	}
	return nil
}

// tokenizeEnvironmentArgs splits a TF_CLI_ARGS value the way the engine does:
// spaces, tabs, and newlines separate tokens outside quotes, and a malformed
// value (for example an unterminated quote) is an error because the engine
// refuses to run with it. The shared parseArgs only splits on spaces, so a tab
// before -out would hide the flag from the check.
func tokenizeEnvironmentArgs(value string) ([]string, error) {
	return shlex.Split(value)
}

// planEnvironment lists the environment layers a plan subprocess receives, in precedence order.
func planEnvironment(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) []string {
	base := info.SanitizedEnv
	if base == nil {
		base = os.Environ()
	}
	entries := slices.Clone(base)
	if atmosConfig != nil {
		entries = envpkg.MergeGlobalEnv(entries, atmosConfig.Env)
	}
	entries = append(entries, info.ComponentEnvList...)
	if info.ComponentExecution == nil {
		return entries
	}
	for i := range info.ComponentExecution.ExtraArguments {
		item := &info.ComponentExecution.ExtraArguments[i]
		if !slices.Contains(item.Commands, subcommandPlan) {
			continue
		}
		for name, value := range item.Environment {
			entries = append(entries, name+"="+value)
		}
	}
	return entries
}

// containsOutputFlag reports any plan output flag, with or without a value.
func containsOutputFlag(args []string) bool {
	return slices.ContainsFunc(args, func(arg string) bool {
		return arg == "-out" || arg == "--out" || strings.HasPrefix(arg, "-out=") || strings.HasPrefix(arg, "--out=")
	})
}

func plannedOutputPath(directory string, args []string) string {
	for i, arg := range args {
		path := ""
		switch {
		case (arg == "-out" || arg == "--out") && i+1 < len(args):
			path = args[i+1]
		case strings.HasPrefix(arg, "-out="):
			path = strings.TrimPrefix(arg, "-out=")
		case strings.HasPrefix(arg, "--out="):
			path = strings.TrimPrefix(arg, "--out=")
		}
		if path != "" {
			if filepath.IsAbs(path) {
				return path
			}
			return filepath.Join(directory, path)
		}
	}
	return ""
}
