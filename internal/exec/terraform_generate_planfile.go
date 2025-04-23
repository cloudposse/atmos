package exec

import (
	"fmt"
	"os"
	"path/filepath"

	log "github.com/charmbracelet/log"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	ErrInvalidFormat                      = errors.New("invalid format")
	ErrCreatingTempDirectory              = errors.New("error creating temporary directory")
	ErrCreatingIntermediateSubdirectories = errors.New("error creating intermediate subdirectories")
	ErrGettingJsonForPlanfile             = errors.New("error getting JSON for planfile")
	ErrConvertingJsonToGoType             = errors.New("error converting JSON to Go type")
)

// PlanfileOptions holds the options for generating a Terraform planfile
type PlanfileOptions struct {
	Component            string
	Stack                string
	Format               string
	File                 string
	ProcessTemplates     bool
	ProcessYamlFunctions bool
	Skip                 []string
}

// ExecuteTerraformGeneratePlanfileCmd executes `terraform generate planfile` command.
func ExecuteTerraformGeneratePlanfileCmd(cmd *cobra.Command, args []string) error {
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

	component := args[0]

	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	options := PlanfileOptions{
		Component:            component,
		Stack:                stack,
		Format:               format,
		File:                 file,
		ProcessTemplates:     processTemplates,
		ProcessYamlFunctions: processYamlFunctions,
		Skip:                 skip,
	}

	return ExecuteTerraformGeneratePlanfile(options, &info)
}

// ExecuteTerraformGeneratePlanfile executes `terraform generate planfile`.
func ExecuteTerraformGeneratePlanfile(
	options PlanfileOptions,
	info *schema.ConfigAndStacksInfo,
) error {
	if options.Format == "" {
		options.Format = "json"
	}

	if options.Format != "json" && options.Format != "yaml" {
		return fmt.Errorf("%w: %s. Supported formats are 'json' and 'yaml'", ErrInvalidFormat, options.Format)
	}

	info.ComponentFromArg = options.Component
	info.Stack = options.Stack
	info.ComponentType = "terraform"
	info.NeedHelp = false

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}

	*info, err = ProcessStacks(atmosConfig, *info, true, options.ProcessTemplates, options.ProcessYamlFunctions, options.Skip)
	if err != nil {
		return err
	}

	componentPath := filepath.Join(atmosConfig.TerraformDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)

	// Create a temporary directory for all temporary files
	tmpDir, err := os.MkdirTemp("", "atmos-terraform-generate-planfile")
	if err != nil {
		return fmt.Errorf(ErrWrappingFormat, ErrCreatingTempDirectory, err)
	}

	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			log.Warn("Error removing temporary directory", "path", path, "error", err)
		}
	}(tmpDir)

	planFile, err := generateNewPlanFile(&atmosConfig, info, componentPath, tmpDir)
	if err != nil {
		return err
	}

	// Get the JSON representation of the new plan
	planJSON, err := getTerraformPlanJSON(&atmosConfig, info, componentPath, planFile)
	if err != nil {
		return fmt.Errorf(ErrWrappingFormat, ErrGettingJsonForPlanfile, err)
	}

	var planFilePath string
	if options.File != "" {
		if filepath.IsAbs(options.File) {
			planFilePath = options.File
		} else {
			planFilePath = filepath.Join(componentPath, options.File)
		}
	} else {
		planFilePath = fmt.Sprintf("%s.%s", constructTerraformComponentPlanfilePath(atmosConfig, *info), options.Format)
	}

	err = u.EnsureDir(planFilePath)
	if err != nil {
		return fmt.Errorf(ErrWrappingFormat, ErrCreatingIntermediateSubdirectories, err)
	}

	log.Debug("Writing the planfile", "file", planFilePath)

	d, err := u.ConvertFromJSON(planJSON)
	if err != nil {
		return fmt.Errorf(ErrWrappingFormat, ErrConvertingJsonToGoType, err)
	}

	const fileMode = 0o644
	if options.Format == "json" {
		err = u.WriteToFileAsJSON(planFilePath, d, fileMode)
	} else {
		err = u.WriteToFileAsYAML(planFilePath, d, fileMode)
	}

	if err != nil {
		return err
	}

	return nil
}
