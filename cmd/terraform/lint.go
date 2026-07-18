package terraform

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

var lintParser *flags.StandardParser

// lintCmd represents the Terraform-aware TFLint command. Unlike a Terraform
// passthrough command, it selects component source directories and runs TFLint
// exactly once for each component.
var lintCmd = &cobra.Command{
	Use:   "lint [component]",
	Short: "Lint Terraform components with TFLint",
	Long: `Lint Terraform components with TFLint.

Without a component, lint checks every Terraform component once. If the same
component is used by several stacks, it is still linted only once. A component
local .tflint.hcl takes precedence over components.terraform.lint.config.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTerraformLint,
}

func init() {
	lintParser = flags.NewStandardParser(
		flags.WithBoolFlag("affected", "", false, "Lint affected Terraform components"),
		flags.WithBoolFlag("all", "", false, "Lint all Terraform components (the default when no component is provided)"),
		flags.WithEnvVars("affected", "ATMOS_TERRAFORM_LINT_AFFECTED"),
		flags.WithEnvVars("all", "ATMOS_TERRAFORM_LINT_ALL"),
	)
	lintParser.RegisterFlags(lintCmd)
	if err := lintParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
	RegisterTerraformCompletions(lintCmd)
	terraformCmd.AddCommand(lintCmd)
}

func runTerraformLint(cmd *cobra.Command, args []string) error {
	if err := internal.ValidateAtmosConfig(); err != nil {
		return err
	}
	v := viper.GetViper()
	if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}
	if err := lintParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	info, err := e.ProcessCommandLineArgs("terraform", terraformCmd, append([]string{"lint"}, args...), nil)
	if err != nil {
		return err
	}
	info.SubCommand = "lint"
	info.ProcessTemplates = v.GetBool("process-templates")
	info.ProcessFunctions = v.GetBool("process-functions")
	info.Skip = v.GetStringSlice("skip")
	info.All = v.GetBool("all")
	info.Affected = v.GetBool("affected")
	// No component is the documented --all default. Keep this on the execution
	// info as well, so callers and future integrations observe the same mode
	// whether it was explicit or implicit.
	if info.ComponentFromArg == "" && !info.Affected {
		info.All = true
	}

	if info.ComponentFromArg != "" && (info.All || info.Affected) {
		return fmt.Errorf("component `%s`: %w", info.ComponentFromArg, errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags)
	}
	if info.All && info.Affected {
		return errUtils.ErrInvalidTerraformFlagsWithAffectedFlag
	}

	if info.Affected {
		return e.ExecuteTerraformLintAffected(terraformLintAffectedArgs(cmd, &info), &info)
	}
	return e.ExecuteTerraformLint(&info)
}

func terraformLintAffectedArgs(cmd *cobra.Command, info *schema.ConfigAndStacksInfo) *e.DescribeAffectedCmdArgs {
	getString := func(name string) string {
		value, _ := cmd.Flags().GetString(name)
		return value
	}
	getBool := func(name string) bool {
		value, _ := cmd.Flags().GetBool(name)
		return value
	}
	return &e.DescribeAffectedCmdArgs{
		RepoPath:             getString("repo-path"),
		Ref:                  getString("ref"),
		SHA:                  getString("sha"),
		SSHKeyPath:           getString("ssh-key"),
		SSHKeyPassword:       getString("ssh-key-password"),
		CloneTargetRef:       getBool("clone-target-ref"),
		IncludeDependents:    getBool("include-dependents"),
		Stack:                info.Stack,
		ProcessTemplates:     info.ProcessTemplates,
		ProcessYamlFunctions: info.ProcessFunctions,
		Skip:                 info.Skip,
		AuthDisabled:         info.AuthDisabled,
	}
}
