package exec

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/generator"
	rp "github.com/cloudposse/atmos/pkg/generator/required_providers"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ExecuteTerraformGenerateRequiredProvidersCmd executes `terraform generate required-providers` command.
// This generates a terraform_override.tf.json file with required_version and required_providers
// blocks from stack configuration (DEV-3124).
func ExecuteTerraformGenerateRequiredProvidersCmd(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGenerateRequiredProvidersCmd")()

	if len(args) != 1 {
		return errors.New("invalid arguments. The command requires one argument `component`")
	}

	flags := cmd.Flags()

	stack, err := flags.GetString("stack")
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

	info.ComponentFromArg = component
	info.Stack = stack
	info.ComponentType = "terraform"
	info.CliArgs = []string{"terraform", "generate", "required-providers"}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	info, err = ProcessStacks(&atmosConfig, info, true, processTemplates, processYamlFunctions, skip, nil)
	if err != nil {
		return err
	}

	// Get the output file path.
	var filePath string
	fileFromArg, err := flags.GetString("file")
	if err != nil {
		fileFromArg = ""
	}

	// Determine working directory.
	workingDir := constructTerraformComponentWorkingDir(&atmosConfig, &info)

	if len(fileFromArg) > 0 {
		filePath = fileFromArg
	} else {
		filePath = filepath.Join(workingDir, rp.DefaultFilenameConst)
	}

	// Check if there's anything to generate.
	if info.RequiredVersion == "" && len(info.RequiredProviders) == 0 {
		log.Info("No required_version or required_providers configured for component",
			"component", info.ComponentFromArg, "stack", info.Stack)
		return nil
	}

	log.Debug("Generating required_providers file",
		"component", info.ComponentFromArg,
		"stack", info.Stack,
		"required_version", info.RequiredVersion,
		"required_providers", info.RequiredProviders)

	log.Debug("Writing the required_providers to file", "file", filePath)

	if info.DryRun {
		return nil
	}

	// Create generator context and generate.
	genCtx := generator.NewGeneratorContext(&atmosConfig, &info, workingDir)

	// Use a custom writer that writes to the specified file path.
	return generator.Generate(context.Background(), rp.Name, genCtx, generator.NewFileWriter())
}
