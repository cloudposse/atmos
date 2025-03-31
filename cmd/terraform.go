package cmd

import (
	"errors"
	"fmt"

	l "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	cfg "github.com/cloudposse/atmos/pkg/config"
	terrerrors "github.com/cloudposse/atmos/pkg/errors"
	h "github.com/cloudposse/atmos/pkg/hooks"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
)

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:                "terraform",
	Aliases:            []string{"tf"},
	Short:              "Execute Terraform commands (e.g., plan, apply, destroy) using Atmos stack configurations",
	Long:               `This command allows you to execute Terraform commands, such as plan, apply, and destroy, using Atmos stack configurations for consistent infrastructure management.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true
	terraformCmd.PersistentFlags().Bool("", false, doubleDashHint)
	addTerraformCommandConfig()
	AddStackCompletion(terraformCmd)
	attachTerraformCommands(terraformCmd)
	RootCmd.AddCommand(terraformCmd)
}

func addTerraformCommandConfig() {
	config.DefaultConfigHandler.AddConfig(terraformCmd, cfg.ConfigOptions{
		FlagName:     "terraform-command",
		EnvVar:       "ATMOS_COMPONENTS_TERRAFORM_COMMAND",
		Description:  "Specifies the executable to be called by `atmos` when running Terraform commands.",
		Key:          "components.terraform.command",
		DefaultValue: "terraform",
	})
	config.DefaultConfigHandler.AddConfig(terraformCmd, cfg.ConfigOptions{
		FlagName:     "terraform-dir",
		EnvVar:       "ATMOS_COMPONENTS_TERRAFORM_BASE_PATH",
		Description:  "Specifies the directory where Terraform commands are executed.",
		Key:          "components.terraform.base_path",
		DefaultValue: "",
	})
	config.DefaultConfigHandler.AddConfig(terraformCmd, cfg.ConfigOptions{
		FlagName:     "terraform-base-path",
		EnvVar:       "ATMOS_COMPONENTS_TERRAFORM_BASE_PATH",
		Description:  "Specifies the directory where Terraform commands are executed.",
		Key:          "components.terraform.base_path",
		DefaultValue: "",
	})
	config.DefaultConfigHandler.BindEnv("components.terraform.apply_auto_approve", "ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE")
	config.DefaultConfigHandler.AddConfig(terraformCmd, cfg.ConfigOptions{
		Key:          "components.terraform.deploy_run_init",
		EnvVar:       "ATMOS_COMPONENTS_TERRAFORM_DEPLOY_RUN_INIT",
		FlagName:     "deploy-run-init",
		Description:  "Run `terraform init` before running `terraform apply`",
		DefaultValue: false,
	})

	config.DefaultConfigHandler.AddConfig(terraformCmd, cfg.ConfigOptions{
		Key:          "components.terraform.init_run_config",
		EnvVar:       "ATMOS_COMPONENTS_TERRAFORM_INIT_RUN_RECONFIGURE",
		FlagName:     "init-run-reconfigure",
		Description:  "Run `terraform init` with reconfigure before running `terraform apply`",
		DefaultValue: false,
	})
	config.DefaultConfigHandler.AddConfig(terraformCmd, cfg.ConfigOptions{
		Key:          "components.terraform.auto_generate_backend_file",
		EnvVar:       "ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE",
		FlagName:     "auto-generate-backend-file",
		Description:  "Automatically generate a backend file for Terraform commands",
		DefaultValue: false,
	})
	config.DefaultConfigHandler.AddConfig(terraformCmd, cfg.ConfigOptions{
		Key:         "components.terraform.append_user_agent",
		FlagName:    "append-user-agent",
		EnvVar:      "ATMOS_COMPONENTS_TERRAFORM_APPEND_USER_AGENT",
		Description: fmt.Sprintf("Sets the TF_APPEND_USER_AGENT environment variable to customize the User-Agent string in Terraform provider requests. Example: `Atmos/%s (Cloud Posse; +https://atmos.tools)`. This flag works with almost all commands.", version.Version),
	})
	config.DefaultConfigHandler.AddConfig(terraformCmd, cfg.ConfigOptions{
		Key:          "components.terraform.skip_init",
		FlagName:     "skip-init",
		Description:  "Skip running `terraform init` before executing terraform commands",
		DefaultValue: false,
	})
	config.DefaultConfigHandler.AddConfig(terraformCmd, cfg.ConfigOptions{
		Key:          "components.terraform.process_templates",
		FlagName:     "process-templates",
		Description:  "Enable/disable Go template processing in Atmos stack manifests when executing terraform commands",
		DefaultValue: true,
	})
	config.DefaultConfigHandler.AddConfig(terraformCmd, cfg.ConfigOptions{
		Key:          "components.terraform.process_functions",
		FlagName:     "process-functions",
		Description:  "Enable/disable YAML functions processing in Atmos stack manifests when executing terraform commands",
		DefaultValue: true,
	})
	config.DefaultConfigHandler.AddConfig(terraformCmd, cfg.ConfigOptions{
		Key:          "components.terraform.skip",
		FlagName:     "skip",
		Description:  "Skip executing specific YAML functions in the Atmos stack manifests when executing terraform commands",
		DefaultValue: []string{},
	})

}

func runHooks(event h.HookEvent, cmd *cobra.Command, args []string) error {
	info := getConfigAndStacksInfo("terraform", cmd, append([]string{cmd.Name()}, args...))

	// Initialize the CLI config
	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return fmt.Errorf("error initializing CLI config: %w", err)
	}

	hooks, err := h.GetHooks(&atmosConfig, &info)
	if err != nil {
		return fmt.Errorf("error getting hooks: %w", err)
	}

	if hooks.HasHooks() {
		l.Info("running hooks", "event", event)
		return hooks.RunAll(event, &atmosConfig, &info, cmd, args)
	}

	return nil
}

func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) error {
	info := getConfigAndStacksInfo("terraform", cmd, args)
	if info.NeedHelp {
		err := actualCmd.Usage()
		if err != nil {
			u.LogErrorAndExit(err)
		}
		return nil
	}

	flags := cmd.Flags()

	processTemplates, err := flags.GetBool("process-templates")
	if err != nil {
		u.PrintErrorMarkdownAndExit("", err, "")
	}

	processYamlFunctions, err := flags.GetBool("process-functions")
	if err != nil {
		u.PrintErrorMarkdownAndExit("", err, "")
	}

	skip, err := flags.GetStringSlice("skip")
	if err != nil {
		u.PrintErrorMarkdownAndExit("", err, "")
	}

	info.ProcessTemplates = processTemplates
	info.ProcessFunctions = processYamlFunctions
	info.Skip = skip

	err = e.ExecuteTerraform(info)
	// For plan-diff, ExecuteTerraform will call OsExit directly if there are differences
	// So if we get here, it means there were no differences or there was an error
	if err != nil {
		if errors.Is(err, terrerrors.ErrPlanHasDiff) {
			// Print the error message but return the error to be handled by main.go
			u.PrintErrorMarkdown("", err, "")
			return err
		}
		// For other errors, continue with existing behavior
		u.PrintErrorMarkdownAndExit("", err, "")
	}
	return nil
}
