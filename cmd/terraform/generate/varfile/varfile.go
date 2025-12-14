package varfile

import (
	"errors"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewVarfileCommand creates the varfile command using modern patterns.
// This command benefits from proper I/O context initialization in root.go PersistentPreRun.
func NewVarfileCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "varfile <component>",
		Short:              "Generate a varfile for a Terraform component",
		Long:               "This command generates a `varfile` for a specified Atmos Terraform component.",
		Args:               cobra.ExactArgs(1),
		FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
		RunE:               executeVarfileCmd,
	}

	// Add flags.
	cmd.Flags().StringP("stack", "s", "", "The stack to use for component generation")
	_ = cmd.MarkFlagRequired("stack")
	cmd.Flags().StringP("file", "f", "", "Specify the path to the varfile to generate for the specified Terraform component in the given stack")

	return cmd
}

func executeVarfileCmd(cmd *cobra.Command, args []string) error {
	component := args[0]
	stack, _ := cmd.Flags().GetString("stack")
	file, _ := cmd.Flags().GetString("file")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return errors.Join(errUtils.ErrInitializeCLIConfig, err)
	}

	opts := &e.VarfileOptions{
		Component: component,
		Stack:     stack,
		File:      file,
		ProcessingOptions: e.ProcessingOptions{
			ProcessTemplates: true,
			ProcessFunctions: true,
			Skip:             nil,
		},
	}
	return e.ExecuteGenerateVarfile(opts, &atmosConfig)
}
