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
	ErrInvalidFormat          = errors.New("invalid format")
	ErrCreatingTempDirectory  = errors.New("error creating temporary directory")
	ErrGettingJsonForPlanfile = errors.New("error getting JSON for planfile")
	ErrConvertingJsonToGoType = errors.New("error converting JSON to Go type")
)

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

	return ExecuteTerraformGeneratePlanfile(component, stack, file, format, processTemplates, processYamlFunctions, skip, info)
}

// ExecuteTerraformGeneratePlanfile executes `terraform generate planfile`.
func ExecuteTerraformGeneratePlanfile(
	component string,
	stack string,
	file string,
	format string,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
	info schema.ConfigAndStacksInfo,
) error {
	if format == "" {
		format = "json"
	}

	if format != "json" && format != "yaml" {
		return fmt.Errorf("%w: %s. Supported formats are 'json' and 'yaml'", ErrInvalidFormat, format)
	}

	info.ComponentFromArg = component
	info.Stack = stack
	info.ComponentType = "terraform"
	info.NeedHelp = false

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	info, err = ProcessStacks(atmosConfig, info, true, processTemplates, processYamlFunctions, skip)
	if err != nil {
		return err
	}

	componentPath := filepath.Join(atmosConfig.TerraformDirAbsolutePath, info.ComponentFolderPrefix, info.FinalComponent)

	// Create a temporary directory for all temporary files
	tmpDir, err := os.MkdirTemp("", "atmos-terraform-generate-planfile")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCreatingTempDirectory, err)
	}

	defer func(path string) {
		err := os.RemoveAll(path)
		if err != nil {
			log.Warn("Error removing temporary directory", "path", path, "error", err)
		}
	}(tmpDir)

	planFile, err := generateNewPlanFile(&atmosConfig, &info, componentPath, tmpDir)
	if err != nil {
		return err
	}

	// Get the JSON representation of the new plan
	planJSON, err := getTerraformPlanJSON(&atmosConfig, &info, componentPath, planFile)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrGettingJsonForPlanfile, err)
	}

	var planFilePath string
	if file != "" {
		if filepath.IsAbs(file) {
			planFilePath = file
		} else {
			planFilePath = filepath.Join(componentPath, file)
		}
	} else {
		planFilePath = fmt.Sprintf("%s.%s", constructTerraformComponentPlanfilePath(atmosConfig, info), format)
	}

	log.Debug("Writing the planfile", "file", planFilePath)

	d, err := u.ConvertFromJSON(planJSON)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrConvertingJsonToGoType, err)
	}

	if format == "json" {
		err = u.WriteToFileAsJSON(planFilePath, d, 0o644)
	} else {
		err = u.WriteToFileAsYAML(planFilePath, d, 0o644)
	}

	if err != nil {
		return err
	}

	return nil
}
