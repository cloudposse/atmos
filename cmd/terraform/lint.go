package terraform

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/scanners/tflint"
	"github.com/cloudposse/atmos/pkg/schema"
)

var lintParser *flags.StandardParser

var executeTerraformLint = tflint.Execute

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
	if err := bindTerraformLintFlags(cmd, v); err != nil {
		return err
	}

	info, err := resolveTerraformLintInfo(v, args)
	if err != nil {
		return err
	}

	if err := checkTerraformLintFlags(info); err != nil {
		return err
	}

	if info.Affected {
		return executeTerraformLint(context.Background(), terraformLintRuntime(), info, terraformLintAffectedArgs(cmd, info))
	}
	return executeTerraformLint(context.Background(), terraformLintRuntime(), info, nil)
}

// bindTerraformLintFlags binds both the shared terraform flags and the
// lint-specific flags to viper so precedence (CLI flags > env vars > config)
// is resolved consistently before the lint run info is assembled.
func bindTerraformLintFlags(cmd *cobra.Command, v *viper.Viper) error {
	if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}
	return lintParser.BindFlagsToViper(cmd, v)
}

// resolveTerraformLintInfo builds the ConfigAndStacksInfo for the lint run,
// applying the documented --all default when neither a component nor
// --affected was given.
func resolveTerraformLintInfo(v *viper.Viper, args []string) (*schema.ConfigAndStacksInfo, error) {
	info, err := e.ProcessCommandLineArgs("terraform", terraformCmd, append([]string{"lint"}, args...), nil)
	if err != nil {
		return nil, err
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
	return &info, nil
}

// checkTerraformLintFlags rejects flag combinations that don't make sense
// together: a single component with a multi-component flag, or --all with
// --affected.
func checkTerraformLintFlags(info *schema.ConfigAndStacksInfo) error {
	if info.ComponentFromArg != "" && (info.All || info.Affected) {
		return fmt.Errorf("component `%s`: %w", info.ComponentFromArg, errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags)
	}
	if info.All && info.Affected {
		return errUtils.ErrInvalidTerraformFlagsWithAffectedFlag
	}
	return nil
}

func terraformLintAffectedArgs(cmd *cobra.Command, info *schema.ConfigAndStacksInfo) *tflint.AffectedOptions {
	getString := func(name string) string {
		value, _ := cmd.Flags().GetString(name)
		return value
	}
	getBool := func(name string) bool {
		value, _ := cmd.Flags().GetBool(name)
		return value
	}
	return &tflint.AffectedOptions{
		RepoPath:             getString("repo-path"),
		Ref:                  getString("ref"),
		SHA:                  getString("sha"),
		SSHKeyPath:           getString("ssh-key"),
		SSHKeyPassword:       getString("ssh-key-password"),
		CloneTargetRef:       getBool("clone-target-ref"),
		Stack:                info.Stack,
		ProcessTemplates:     info.ProcessTemplates,
		ProcessYamlFunctions: info.ProcessFunctions,
		Skip:                 info.Skip,
		AuthDisabled:         info.AuthDisabled,
	}
}

func terraformLintRuntime() *tflint.Runtime {
	return &tflint.Runtime{
		SetupAuth:          e.SetupComponentAuthForCLI,
		DescribeStacks:     e.ExecuteDescribeStacksWithAuthDisabled,
		ProcessStacks:      e.ProcessStacks,
		AffectedComponents: resolveTerraformLintAffectedComponents,
	}
}

// resolveTerraformLintAffectedComponents reuses the same git-target dispatch
// `atmos describe affected`/`--affected` already uses (e.GetAffectedComponents)
// instead of re-implementing the RepoPath/CloneTargetRef/ref-checkout switch
// here. Terraform-only/not-deleted filtering is deliberately NOT duplicated
// here — pkg/scanners/tflint/command.go's executeAffected already applies its
// own filterAffected to whatever any Runtime.AffectedComponents implementation
// returns (see TestExecuteAffectedFiltersAndDeduplicatesTargets), so this
// adapter can stay a plain passthrough.
func resolveTerraformLintAffectedComponents(atmosConfig *schema.AtmosConfiguration, options *tflint.AffectedOptions, authManager auth.AuthManager) ([]schema.Affected, error) {
	return e.GetAffectedComponents(&e.DescribeAffectedCmdArgs{
		CLIConfig:            atmosConfig,
		RepoPath:             options.RepoPath,
		CloneTargetRef:       options.CloneTargetRef,
		Ref:                  options.Ref,
		SHA:                  options.SHA,
		SSHKeyPath:           options.SSHKeyPath,
		SSHKeyPassword:       options.SSHKeyPassword,
		TargetBranch:         options.TargetBranch,
		IncludeSettings:      options.IncludeSettings,
		Stack:                options.Stack,
		ProcessTemplates:     options.ProcessTemplates,
		ProcessYamlFunctions: options.ProcessYamlFunctions,
		Skip:                 options.Skip,
		ExcludeLocked:        options.ExcludeLocked,
		AuthManager:          authManager,
		AuthDisabled:         options.AuthDisabled,
	})
}
