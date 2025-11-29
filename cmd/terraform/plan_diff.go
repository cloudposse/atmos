package terraform

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// planDiffParser handles flag parsing for plan-diff command.
var planDiffParser *flags.StandardParser

// planDiffCmd represents the terraform plan-diff command (custom Atmos command).
var planDiffCmd = &cobra.Command{
	Use:   "plan-diff <component>",
	Short: "Compare two Terraform plans and show the differences",
	Long: `Compare two Terraform plans and show the differences between them.

It takes an original plan file (--orig) and optionally a new plan file (--new).
If the new plan file is not provided, it will generate one by running 'terraform plan'
with the current configuration.

The command shows differences in variables, resources, and outputs between the two plans.

Example usage:
  atmos terraform plan-diff myapp -s dev --orig=orig.plan
  atmos terraform plan-diff myapp -s dev --orig=orig.plan --new=new.plan`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]

		// Use Viper to respect precedence (flag > env > config > default)
		v := viper.GetViper()

		// Bind terraform flags (--stack, etc.) to Viper
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Bind plan-diff specific flags to Viper
		if err := planDiffParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get flag values from Viper
		stack := v.GetString("stack")
		orig := v.GetString("orig")
		newPlan := v.GetString("new")

		// Validate required flags
		if stack == "" {
			return fmt.Errorf("stack is required (use --stack or -s)")
		}
		if orig == "" {
			return fmt.Errorf("original plan file is required (use --orig)")
		}

		// Initialize Atmos configuration
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
		if err != nil {
			return err
		}

		// Build ConfigAndStacksInfo for TerraformPlanDiff
		info := &schema.ConfigAndStacksInfo{
			ComponentFromArg: component,
			Stack:            stack,
			SubCommand:       "plan-diff",
			ComponentType:    cfg.TerraformComponentType,
		}

		// Store plan file paths in AdditionalArgsAndFlags for backward compatibility
		info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, "--orig="+orig)
		if newPlan != "" {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, "--new="+newPlan)
		}

		return e.TerraformPlanDiff(&atmosConfig, info)
	},
}

func init() {
	// Create parser with plan-diff specific flags using functional options.
	planDiffParser = flags.NewStandardParser(
		flags.WithStringFlag("orig", "", "", "Path to the original Terraform plan file (required)"),
		flags.WithStringFlag("new", "", "", "Path to the new Terraform plan file (optional)"),
		flags.WithEnvVars("orig", "ATMOS_TERRAFORM_PLAN_DIFF_ORIG"),
		flags.WithEnvVars("new", "ATMOS_TERRAFORM_PLAN_DIFF_NEW"),
	)

	// Register flags with the command as persistent flags.
	planDiffParser.RegisterPersistentFlags(planDiffCmd)

	// Bind flags to Viper for environment variable support.
	if err := planDiffParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Mark orig as required.
	err := planDiffCmd.MarkPersistentFlagRequired("orig")
	if err != nil {
		errUtils.CheckErrorPrintAndExit(err, "Error marking 'orig' flag as required", "")
	}

	// Register completions for planDiffCmd.
	RegisterTerraformCompletions(planDiffCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(planDiffCmd)
}
