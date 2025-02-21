package cmd

import (
	"errors"
	"fmt"

	l "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	h "github.com/cloudposse/atmos/pkg/hooks"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// terraformCmd represents the base command for all terraform sub-commands
var terraformCmd = &cobra.Command{
	Use:                "terraform",
	Aliases:            []string{"tf"},
	Short:              "Execute Terraform commands (e.g., plan, apply, destroy) using Atmos stack configurations",
	Long:               `This command allows you to execute Terraform commands, such as plan, apply, and destroy, using Atmos stack configurations for consistent infrastructure management.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Example:            terraformUsage,
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	terraformCmd.DisableFlagParsing = true

	terraformCmd.PersistentFlags().StringP("stack", "s", "", "atmos terraform <terraform_command> <component> -s <stack>")
	terraformCmd.PersistentFlags().Bool("", false, doubleDashHint)

	// Flags related to `--affected` flag (similar to `atmos describe affected`)
	// These flags are only used then executing `atmos terraform plan/apply/deploy --affected`
	terraformCmd.PersistentFlags().String("repo-path", "", "Filesystem path to the already cloned target repository with which to compare the current branch: atmos terraform <sub-command> --affected --repo-path <path_to_already_cloned_repo>")
	terraformCmd.PersistentFlags().String("ref", "", "Git reference with which to compare the current branch: atmos terraform <sub-command> --affected --ref refs/heads/main. Refer to https://git-scm.com/book/en/v2/Git-Internals-Git-References for more details")
	terraformCmd.PersistentFlags().String("sha", "", "Git commit SHA with which to compare the current branch: atmos terraform <sub-command> --affected --sha 3a5eafeab90426bd82bf5899896b28cc0bab3073")
	terraformCmd.PersistentFlags().String("ssh-key", "", "Path to PEM-encoded private key to clone private repos using SSH: atmos terraform <sub-command> --affected --ssh-key <path_to_ssh_key>")
	terraformCmd.PersistentFlags().String("ssh-key-password", "", "Encryption password for the PEM-encoded private key if the key contains a password-encrypted PEM block: atmos terraform <sub-command> --affected --ssh-key <path_to_ssh_key> --ssh-key-password <password>")
	terraformCmd.PersistentFlags().Bool("include-dependents", false, "Include the dependent components and process them in the dependency order: atmos terraform <sub-command> --affected --include-dependents=true")
	terraformCmd.PersistentFlags().Bool("clone-target-ref", false, "Clone the target reference with which to compare the current branch: atmos terraform <sub-command> --affected --clone-target-ref=true\n"+
		"If set to 'false' (default), the target reference will be checked out instead\n"+
		"This requires that the target reference is already cloned by Git, and the information about it exists in the '.git' directory")

	AddStackCompletion(terraformCmd)
	attachTerraformCommands(terraformCmd)
	RootCmd.AddCommand(terraformCmd)
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

func terraformRun(cmd *cobra.Command, actualCmd *cobra.Command, args []string) {
	info := getConfigAndStacksInfo("terraform", cmd, args)

	if info.NeedHelp {
		err := actualCmd.Usage()
		if err != nil {
			l.Fatal(err)
		}
		return
	}

	if info.Affected && info.All {
		err := errors.New("only one of '--affected' or '--all' flag can be specified")
		u.PrintErrorMarkdownAndExit("", err, "")
	}

	if info.Affected {
		err := e.ExecuteTerraformAffected(cmd, args, info)
		if err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}
		return
	}

	if info.All {
		err := e.ExecuteTerraformAll(cmd, args, info)
		if err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}
		return
	}

	err := e.ExecuteTerraform(info)
	if err != nil {
		u.PrintErrorMarkdownAndExit("", err, "")
	}
}
