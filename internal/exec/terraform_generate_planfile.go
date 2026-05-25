package exec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	reusePlanStr, err := flags.GetString("reuse-plan")
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
		ReusePlan:            tf.ReusePlanMode(reusePlanStr),
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

	// Resolve the source planfile to convert. With ReusePlanNever (the default
	// and historical behavior), a fresh plan is generated in a temp dir. With
	// ReusePlanAuto/ReusePlanAlways, we attempt to reuse the canonical binary
	// planfile produced by a prior `atmos terraform plan`.
	planFile, cleanup, err := resolveSourcePlanFile(&atmosConfig, info, componentPath, options.ReusePlan)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Get the JSON representation of the plan.
	planJSON, err := getTerraformPlanJSON(&atmosConfig, info, componentPath, planFile)
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

// resolveSourcePlanFile returns the path to the binary planfile that
// `terraform show -json` will run against. The cleanup function, when non-nil,
// must be invoked by the caller to release temp-dir resources.
//
// Behavior by ReusePlanMode:
//   - "" (zero value) and "never": always generate a fresh plan into a temp dir
//     (the historical behavior).
//   - "auto": reuse the canonical binary planfile if it exists and the staleness
//     gates pass; otherwise fall back to a fresh plan.
//   - "always": reuse the canonical binary planfile or return
//     ErrReusePlanUnavailable if it is missing or stale.
//
// Any other value returns ErrReusePlanInvalidMode.
func resolveSourcePlanFile(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	componentPath string,
	mode tf.ReusePlanMode,
) (string, func(), error) {
	switch mode {
	case "", tf.ReusePlanNever:
		return generateFreshPlanFile(atmosConfig, info, componentPath)

	case tf.ReusePlanAuto:
		canonical := constructTerraformComponentPlanfilePath(atmosConfig, info)
		reason := planfileReuseGateReason(atmosConfig, canonical, componentPath)
		if reason == "" {
			log.Debug("Reusing existing planfile", "path", canonical, "mode", string(mode))
			return canonical, nil, nil
		}
		log.Debug("Falling back to fresh plan",
			"path", canonical, "mode", string(mode), "reason", reason)
		return generateFreshPlanFile(atmosConfig, info, componentPath)

	case tf.ReusePlanAlways:
		canonical := constructTerraformComponentPlanfilePath(atmosConfig, info)
		if reason := planfileReuseGateReason(atmosConfig, canonical, componentPath); reason != "" {
			return "", nil, fmt.Errorf("%w: %s", errUtils.ErrReusePlanUnavailable, reason)
		}
		log.Debug("Reusing existing planfile", "path", canonical, "mode", string(mode))
		return canonical, nil, nil

	default:
		return "", nil, fmt.Errorf("%w: %q (expected one of: never, auto, always)",
			errUtils.ErrReusePlanInvalidMode, string(mode))
	}
}

// generateFreshPlanFile runs `terraform plan -out=...` into a temp directory
// and returns the resulting binary path with a cleanup function that removes
// the temp dir.
func generateFreshPlanFile(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	componentPath string,
) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "atmos-terraform-generate-planfile")
	if err != nil {
		return "", nil, errors.Join(ErrCreatingTempDirectory, err)
	}

	cleanup := func() {
		if rmErr := os.RemoveAll(tmpDir); rmErr != nil {
			log.Warn("Error removing temporary directory", "path", tmpDir, "error", rmErr)
		}
	}

	planFile, err := generateNewPlanFile(atmosConfig, info, componentPath, tmpDir)
	if err != nil {
		cleanup()
		return "", nil, err
	}

	return planFile, cleanup, nil
}

// planfileReuseGateReason returns an empty string when reuse of the canonical
// binary planfile is permitted under the current atmosConfig, or a short
// human-readable reason when it is not. It composes the config-level gate
// (SkipPlanfile) with the on-disk staleness gates so callers under
// ReusePlanAuto/ReusePlanAlways see a single decision.
func planfileReuseGateReason(atmosConfig *schema.AtmosConfiguration, planfilePath, componentPath string) string {
	if atmosConfig != nil && atmosConfig.Components.Terraform.Plan.SkipPlanfile {
		return "components.terraform.plan.skip_planfile is true; no binary planfile is ever produced"
	}
	return planfileReuseStaleness(planfilePath, componentPath)
}

// planfileReuseStaleness returns an empty string when the canonical binary
// planfile is fresh enough to reuse, or a short human-readable reason when it
// is not. Reasons are stable strings suitable for logging and for inclusion in
// error messages produced under ReusePlanAlways.
//
// Gates (in order):
//  1. The binary planfile exists.
//  2. The lock file (.terraform.lock.hcl) alongside the binary is not newer
//     than the binary. Looking next to the binary (rather than in componentPath)
//     handles the workdir-provisioner case where terraform init writes the
//     lockfile in the workdir, not in the source component directory.
//  3. No *.tf, *.tf.json, or *.tfvars* file directly in componentPath is newer
//     than the binary. The scan is intentionally one level deep — vendored
//     submodules under ./modules/ are out of scope, and the caller is
//     responsible for invalidating reuse when they change.
func planfileReuseStaleness(planfilePath, componentPath string) string {
	pfInfo, err := os.Stat(planfilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "planfile does not exist at " + planfilePath
		}
		return fmt.Sprintf("planfile stat failed: %v", err)
	}
	if pfInfo.IsDir() {
		return "planfile path is a directory: " + planfilePath
	}

	pfMtime := pfInfo.ModTime()

	lockPath := filepath.Join(filepath.Dir(planfilePath), ".terraform.lock.hcl")
	if lockInfo, err := os.Stat(lockPath); err == nil {
		if lockInfo.ModTime().After(pfMtime) {
			return ".terraform.lock.hcl is newer than planfile"
		}
	}

	entries, err := os.ReadDir(componentPath)
	if err != nil {
		return fmt.Sprintf("cannot read component directory %s: %v", componentPath, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isTerraformInputFile(name) {
			continue
		}
		fi, err := entry.Info()
		if err != nil {
			// Be permissive: a transient stat failure shouldn't block reuse.
			continue
		}
		if fi.ModTime().After(pfMtime) {
			return fmt.Sprintf("input file %s is newer than planfile", name)
		}
	}

	return ""
}

// isTerraformInputFile reports whether a filename matches a Terraform input
// shape that should invalidate a stored plan when modified.
func isTerraformInputFile(name string) bool {
	switch {
	case strings.HasSuffix(name, ".tf"),
		strings.HasSuffix(name, ".tf.json"),
		strings.Contains(name, ".tfvars"):
		return true
	}
	return false
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
		planFilePath = fmt.Sprintf("%s.%s", constructTerraformComponentPlanfilePath(atmosConfig, info), format)
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
