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
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// errorModeFlagName is the --error-mode flag's name, referenced from both its
// registration and every place that reads it back via viper.
const errorModeFlagName = "error-mode"

// maxFindingsFlagName is the --max-findings flag's name, referenced from both its
// registration and every place that reads it back via viper.
const maxFindingsFlagName = "max-findings"

// outputFormatFlagName is the --format flag's name. It selects Atmos's
// presentation of TFLint findings; TFLint itself always emits SARIF.
const outputFormatFlagName = "format"

const (
	outputFormatMarkdown = "markdown"
	outputFormatRich     = "rich"
)

// defaultMaxFindings is the default maximum number of individual findings shown
// per component when the user has not passed --max-findings or
// ATMOS_TERRAFORM_LINT_MAX_FINDINGS. Matches pkg/scanners/sarif's own
// defaultMaxFindings.
const defaultMaxFindings = 10

// maxFindingsUnset is the sentinel flag default that signals "user did not pass
// --max-findings and no env override applied," so resolveMaxFindings falls
// through to defaultMaxFindings. A user-supplied 0 means "show every finding"
// (matches cmd/aws/security's identical --max-findings convention) and must be
// distinguishable from "not set at all," which a plain 0 default could not do.
const maxFindingsUnset = -1

// sarifUnlimitedFindings is the sentinel pkg/scanners/sarif's RenderMarkdownOptions
// recognizes as "no cap" (see its MaxFindings doc comment) — distinct from this
// command's own maxFindingsUnset, since sarif's zero value already means "use its
// own default" for every other (non-CLI) caller, like the checkov/trivy/kics hooks
// that never set MaxFindings at all.
const sarifUnlimitedFindings = -1

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
		flags.WithStringFlag(errorModeFlagName, "", "", "How to handle recoverable errors while discovering lint targets (e.g. a Terraform backend not yet provisioned, or a component's identity failing to resolve): warn (degrade + summary, default), silent (degrade, no summary), or strict (fail immediately). Defaults to atmos.yaml's components.terraform.lint.error_mode, or warn"),
		flags.WithIntFlag(maxFindingsFlagName, "", maxFindingsUnset, fmt.Sprintf("Maximum number of individual findings to show per component (0 = unlimited, default %d)", defaultMaxFindings)),
		flags.WithStringFlag(outputFormatFlagName, "", outputFormatMarkdown, "Output format: markdown, rich"),
		flags.WithEnvVars("affected", "ATMOS_TERRAFORM_LINT_AFFECTED"),
		flags.WithEnvVars("all", "ATMOS_TERRAFORM_LINT_ALL"),
		flags.WithEnvVars(errorModeFlagName, "ATMOS_TERRAFORM_LINT_ERROR_MODE"),
		flags.WithEnvVars(maxFindingsFlagName, "ATMOS_TERRAFORM_LINT_MAX_FINDINGS"),
		flags.WithEnvVars(outputFormatFlagName, "ATMOS_TERRAFORM_LINT_FORMAT"),
		flags.WithValidValues(errorModeFlagName, "strict", "warn", "silent"),
		flags.WithValidValues(outputFormatFlagName, outputFormatMarkdown, outputFormatRich),
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

	errorMode := v.GetString(errorModeFlagName)
	maxFindings := resolveMaxFindings(cmd, v.GetInt(maxFindingsFlagName))
	outputFormat := v.GetString(outputFormatFlagName)
	if info.Affected {
		return executeTerraformLint(context.Background(), terraformLintRuntime(errorMode, outputFormat), info, terraformLintAffectedArgs(cmd, info, errorMode), maxFindings)
	}
	return executeTerraformLint(context.Background(), terraformLintRuntime(errorMode, outputFormat), info, nil, maxFindings)
}

// resolveMaxFindings determines the effective max-findings value using flag
// precedence: an explicit user-supplied value (via --max-findings or
// ATMOS_TERRAFORM_LINT_MAX_FINDINGS) wins, then defaultMaxFindings. A
// user-supplied 0 is preserved and translated to sarifUnlimitedFindings — sarif
// itself treats 0 as "unset, use my own default," so the CLI's "0 = unlimited"
// convention (matching cmd/aws/security's --max-findings) needs its own distinct
// sentinel to mean the same thing to the rendering layer.
//
// The flagValue argument is what Viper returned for the flag: maxFindingsUnset
// (-1) when the user did not pass --max-findings and no env override applied, or
// the user/env value otherwise.
func resolveMaxFindings(cmd *cobra.Command, flagValue int) int {
	resolved := defaultMaxFindings
	switch {
	case cmd != nil && cmd.Flags().Changed(maxFindingsFlagName):
		// User explicitly passed --max-findings on the CLI (any value, including 0).
		resolved = flagValue
	case flagValue != maxFindingsUnset:
		// User set ATMOS_TERRAFORM_LINT_MAX_FINDINGS (Viper picked it up via env
		// binding). Treat any value != maxFindingsUnset as an explicit override,
		// including 0.
		resolved = flagValue
	}
	if resolved == 0 {
		return sarifUnlimitedFindings
	}
	return resolved
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

func terraformLintAffectedArgs(cmd *cobra.Command, info *schema.ConfigAndStacksInfo, errorMode string) *tflint.AffectedOptions {
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
		ErrorMode:            errorMode,
	}
}

// terraformLintRuntime builds the Runtime lint executes against. ErrorMode is the raw
// --error-mode flag/env value (possibly empty — resolved against atmos.yaml's
// components.terraform.lint.error_mode, then "warn", once atmosConfig is available inside
// the DescribeStacks closure below).
func terraformLintRuntime(errorMode, outputFormat string) *tflint.Runtime {
	return &tflint.Runtime{
		SetupAuth:          e.SetupComponentAuthForCLI,
		DescribeStacks:     terraformLintDescribeStacks(errorMode),
		ProcessStacks:      e.ProcessStacks,
		AffectedComponents: resolveTerraformLintAffectedComponents,
		OutputFormat:       outputFormat,
	}
}

// terraformLintDescribeStacks wraps e.ExecuteDescribeStacksWithOptions so `atmos terraform
// lint` (no --affected) gets the same --error-mode graceful degradation as `describe
// stacks`/`list stacks` — e.g. an unrelated component's broken identity config no longer
// aborts the whole lint run when error-mode is warn/silent (the mechanism's default). See
// docs/fixes/2026-07-22-terraform-lint-error-mode.md.
func terraformLintDescribeStacks(errorMode string) func(
	*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager, bool,
) (map[string]any, error) {
	return func(
		atmosConfig *schema.AtmosConfiguration,
		filterByStack string,
		components []string,
		componentTypes []string,
		sections []string,
		ignoreMissingFiles bool,
		processTemplates bool,
		processYamlFunctions bool,
		includeEmptyStacks bool,
		skip []string,
		authManager auth.AuthManager,
		authDisabled bool,
	) (map[string]any, error) {
		resolvedMode := e.ResolveErrorMode(errorMode, atmosConfig.Components.Terraform.Lint.ErrorMode)
		errOptions, collector := e.ErrorOptionsFromMode(resolvedMode)

		progress := spinner.New("Discovering Terraform lint targets")
		progress.Start()
		errOptions.OnProgress = func(stackFile string, index, total int) {
			progress.Update(fmt.Sprintf("Discovering Terraform lint targets: %s (%d/%d)", stackFile, index+1, total))
		}

		result, err := e.ExecuteDescribeStacksWithOptions(
			atmosConfig, filterByStack, components, componentTypes, sections,
			ignoreMissingFiles, processTemplates, processYamlFunctions, includeEmptyStacks,
			skip, authManager, authDisabled, errOptions,
		)
		if err != nil {
			progress.Error("Failed to discover Terraform lint targets")
		} else {
			progress.Success(fmt.Sprintf("Discovered Terraform lint targets across %d stack file(s)", len(result)))
		}

		e.PrintErrorModeSummary(resolvedMode, collector)
		return result, err
	}
}

// resolveTerraformLintAffectedComponents reuses the same git-target dispatch
// `atmos describe affected`/`--affected` already uses (e.GetAffectedComponentsWithOptions)
// instead of re-implementing the RepoPath/CloneTargetRef/ref-checkout switch
// here. Terraform-only/not-deleted filtering is deliberately NOT duplicated
// here — pkg/scanners/tflint/command.go's executeAffected already applies its
// own filterAffected to whatever any Runtime.AffectedComponents implementation
// returns (see TestExecuteAffectedFiltersAndDeduplicatesTargets), so this
// adapter can stay a plain passthrough.
func resolveTerraformLintAffectedComponents(atmosConfig *schema.AtmosConfiguration, options *tflint.AffectedOptions, authManager auth.AuthManager) ([]schema.Affected, error) {
	resolvedMode := e.ResolveErrorMode(options.ErrorMode, atmosConfig.Components.Terraform.Lint.ErrorMode)
	errOptions, collector := e.ErrorOptionsFromMode(resolvedMode)
	defer e.PrintErrorModeSummary(resolvedMode, collector)

	return e.GetAffectedComponentsWithOptions(&e.DescribeAffectedCmdArgs{
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
	}, errOptions)
}
