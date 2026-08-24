package exec

import (
	"context"
	"path/filepath"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/generator"
	rp "github.com/cloudposse/atmos/pkg/generator/required_providers"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// requiredProvidersFlags holds the parsed flags for the generate required-providers command.
type requiredProvidersFlags struct {
	stack                string
	processTemplates     bool
	processYamlFunctions bool
	skip                 []string
	file                 string
}

// parseRequiredProvidersFlags extracts flags from the cobra command.
func parseRequiredProvidersFlags(cmd *cobra.Command) (requiredProvidersFlags, error) {
	defer perf.Track(nil, "exec.parseRequiredProvidersFlags")()

	flags := cmd.Flags()
	var f requiredProvidersFlags
	var err error

	if f.stack, err = flags.GetString("stack"); err != nil {
		return f, err
	}
	if f.processTemplates, err = flags.GetBool("process-templates"); err != nil {
		return f, err
	}
	if f.processYamlFunctions, err = flags.GetBool("process-functions"); err != nil {
		return f, err
	}
	if f.skip, err = flags.GetStringSlice("skip"); err != nil {
		return f, err
	}
	f.file, _ = flags.GetString("file") // Ignore error, optional flag.

	return f, nil
}

// ExecuteTerraformGenerateRequiredProvidersCmd executes `terraform generate required-providers` command.
// This generates a terraform_override.tf.json file with required_version and required_providers
// blocks from stack configuration (DEV-3124).
func ExecuteTerraformGenerateRequiredProvidersCmd(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGenerateRequiredProvidersCmd")()

	if len(args) != 1 {
		return errUtils.ErrInvalidComponentArgument
	}

	f, err := parseRequiredProvidersFlags(cmd)
	if err != nil {
		return err
	}

	component := args[0]
	info, err := ProcessCommandLineArgs("terraform", cmd, args, nil)
	if err != nil {
		return err
	}

	info.ComponentFromArg = component
	info.Stack = f.stack
	info.ComponentType = "terraform"
	info.CliArgs = []string{"terraform", "generate", "required-providers"}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	info, err = ProcessStacks(&atmosConfig, info, true, f.processTemplates, f.processYamlFunctions, f.skip, nil)
	if err != nil {
		return err
	}

	return generateRequiredProvidersFile(&atmosConfig, &info, f.file)
}

// generateRequiredProvidersFile creates the required_providers file using the generator.
func generateRequiredProvidersFile(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, fileFromArg string) error {
	defer perf.Track(nil, "exec.generateRequiredProvidersFile")()

	// Check if there's anything to generate.
	if info.RequiredVersion == "" && len(info.RequiredProviders) == 0 {
		log.Info("No required_version or required_providers configured for component",
			"component", info.ComponentFromArg, "stack", info.Stack)
		return nil
	}

	workingDir := constructTerraformComponentWorkingDir(atmosConfig, info)
	genCtx := generator.NewGeneratorContext(atmosConfig, info, workingDir)
	configureCustomFilePath(genCtx, fileFromArg)

	log.Debug("Generating required_providers file",
		"component", info.ComponentFromArg,
		"stack", info.Stack,
		"required_version", info.RequiredVersion,
		"required_providers", info.RequiredProviders)

	if info.DryRun {
		return nil
	}

	return generator.Generate(context.Background(), rp.Name, genCtx, generator.NewFileWriter())
}

// configureCustomFilePath sets up the generator context for a custom file path.
func configureCustomFilePath(genCtx *generator.GeneratorContext, fileFromArg string) {
	if fileFromArg == "" {
		return
	}
	genCtx.CustomFilename = filepath.Base(fileFromArg)
	if dir := filepath.Dir(fileFromArg); dir != "." && dir != "" {
		genCtx.WorkingDir = dir
	}
}
