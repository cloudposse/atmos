package terraform

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Flag name constants for varfile commands.
const (
	flagStack            = "stack"
	flagFile             = "file"
	flagProcessTemplates = "process-templates"
	flagProcessFunctions = "process-functions"
	flagSkip             = "skip"
)

var (
	// VarfileParser handles flag parsing for varfile command.
	varfileParser *flags.StandardParser
	// WriteVarfileParser handles flag parsing for write varfile command.
	writeVarfileParser *flags.StandardParser
)

// VarfileConfig holds the configuration for varfile generation.
type VarfileConfig struct {
	Component        string
	Stack            string
	File             string
	ProcessTemplates bool
	ProcessFunctions bool
	Skip             []string
}

// ParseVarfileFlags extracts varfile configuration from Viper after flag binding.
func ParseVarfileFlags(v *viper.Viper) VarfileConfig {
	return VarfileConfig{
		Stack:            v.GetString(flagStack),
		File:             v.GetString(flagFile),
		ProcessTemplates: v.GetBool(flagProcessTemplates),
		ProcessFunctions: v.GetBool(flagProcessFunctions),
		Skip:             v.GetStringSlice(flagSkip),
	}
}

// ValidateVarfileConfig validates the varfile configuration.
func ValidateVarfileConfig(config *VarfileConfig) error {
	if config.Stack == "" {
		return errUtils.ErrMissingStack
	}
	return nil
}

// ExecuteVarfileGeneration executes the varfile generation with the given config.
func ExecuteVarfileGeneration(config *VarfileConfig) error {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return err
	}
	opts := &e.VarfileOptions{
		Component: config.Component,
		Stack:     config.Stack,
		File:      config.File,
		ProcessingOptions: e.ProcessingOptions{
			ProcessTemplates: config.ProcessTemplates,
			ProcessFunctions: config.ProcessFunctions,
			Skip:             config.Skip,
		},
	}
	return e.ExecuteGenerateVarfile(opts, &atmosConfig)
}

// runVarfileCommand is the shared implementation for varfile commands.
func runVarfileCommand(cmd *cobra.Command, component string, parser *flags.StandardParser, deprecationMsg string) error {
	_ = ui.Warning(deprecationMsg)

	v := viper.GetViper()

	if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	if err := parser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	config := ParseVarfileFlags(v)
	config.Component = component

	if err := ValidateVarfileConfig(&config); err != nil {
		return err
	}

	return ExecuteVarfileGeneration(&config)
}

// varfileCmd represents the terraform varfile command (legacy Atmos command).
// Deprecated: Use 'terraform generate varfile' instead.
var varfileCmd = &cobra.Command{
	Use:                "varfile <component>",
	Short:              "Generate a varfile for a Terraform component (deprecated)",
	Long:               `Generate a varfile for a Terraform component. This command is deprecated in favor of 'terraform generate varfile'.`,
	Args:               cobra.ExactArgs(1),
	Deprecated:         "use 'atmos terraform generate varfile' instead",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runVarfileCommand(cmd, args[0], varfileParser, "'terraform varfile' is deprecated, use 'terraform generate varfile' instead")
	},
}

// writeVarfileCmd represents the terraform write varfile command (legacy Atmos command).
// Deprecated: Use 'terraform generate varfile' instead.
var writeVarfileCmd = &cobra.Command{
	Use:                "write varfile <component>",
	Short:              "Generate a varfile for a Terraform component (deprecated)",
	Long:               `Generate a varfile for a Terraform component. This command is deprecated in favor of 'terraform generate varfile'.`,
	Args:               cobra.ExactArgs(1),
	Deprecated:         "use 'atmos terraform generate varfile' instead",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	RunE: func(cmd *cobra.Command, args []string) error {
		return runVarfileCommand(cmd, args[0], writeVarfileParser, "'terraform write varfile' is deprecated, use 'terraform generate varfile' instead")
	},
}

// createVarfileParser creates a standard parser with varfile-specific flags.
func createVarfileParser() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithStringFlag(flagFile, "f", "", "Path to the varfile to generate"),
		flags.WithBoolFlag(flagProcessTemplates, "", true, "Enable Go template processing in Atmos stack manifests"),
		flags.WithBoolFlag(flagProcessFunctions, "", true, "Enable YAML functions processing in Atmos stack manifests"),
		flags.WithStringSliceFlag(flagSkip, "", []string{}, "Skip processing specific Atmos YAML functions"),
		flags.WithEnvVars(flagFile, "ATMOS_FILE"),
		flags.WithEnvVars(flagProcessTemplates, "ATMOS_PROCESS_TEMPLATES"),
		flags.WithEnvVars(flagProcessFunctions, "ATMOS_PROCESS_FUNCTIONS"),
		flags.WithEnvVars(flagSkip, "ATMOS_SKIP"),
	)
}

func init() {
	// Create parsers using shared factory function.
	varfileParser = createVarfileParser()
	writeVarfileParser = createVarfileParser()

	// Register flags with the commands.
	varfileParser.RegisterFlags(varfileCmd)
	writeVarfileParser.RegisterFlags(writeVarfileCmd)

	// Bind flags to Viper for environment variable support.
	if err := varfileParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
	if err := writeVarfileParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register completions for varfileCmd.
	RegisterTerraformCompletions(varfileCmd)
	RegisterTerraformCompletions(writeVarfileCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(varfileCmd)
	terraformCmd.AddCommand(writeVarfileCmd)
}
