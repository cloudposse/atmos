package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// helmfileCmd represents the base command for all helmfile sub-commands
var helmfileCmd = &cobra.Command{
	Use:                "helmfile",
	Aliases:            []string{"hf"},
	Short:              "Manage Helmfile-based Kubernetes deployments",
	Long:               `This command runs Helmfile commands to manage Kubernetes deployments using Helmfile.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Args:               cobra.NoArgs,
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	helmfileCmd.DisableFlagParsing = true
	helmfileCmd.PersistentFlags().Bool("", false, doubleDashHint)
	config.DefaultConfigHandler.AddConfig(helmfileCmd, config.ConfigOptions{
		FlagName:     "helmfile-command",
		EnvVar:       "ATMOS_COMPONENTS_HELMFILE_COMMAND",
		Description:  "Specifies the executable to be called by `atmos` when running Helmfile commands.",
		Key:          "components.helmfile.command",
		DefaultValue: "helmfile",
	})
	config.DefaultConfigHandler.AddConfig(helmfileCmd, config.ConfigOptions{
		FlagName:     "helmfile-dir",
		EnvVar:       "ATMOS_COMPONENTS_HELMFILE_BASE_PATH",
		Description:  "Specifies the directory where Helmfile commands are executed.",
		Key:          "components.helmfile.base_path",
		DefaultValue: "",
	})
	config.DefaultConfigHandler.AddConfig(helmfileCmd, config.ConfigOptions{
		FlagName:     "helmfile-base-path",
		EnvVar:       "ATMOS_COMPONENTS_HELMFILE_BASE_PATH",
		Description:  "Specifies the directory where Helmfile commands are executed.",
		Key:          "components.helmfile.base_path",
		DefaultValue: "",
	})

	AddStackCompletion(helmfileCmd)
	helmfileCommandConfig()
	RootCmd.AddCommand(helmfileCmd)
}

func helmfileCommandConfig() {
	config.DefaultConfigHandler.SetDefault("components.helmfile.use_eks", true)
	config.DefaultConfigHandler.BindEnv("components.helmfile.use_eks", "ATMOS_COMPONENTS_HELMFILE_USE_EKS")
	config.DefaultConfigHandler.BindEnv("components.helmfile.kubeconfig_path", "ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH")
	config.DefaultConfigHandler.BindEnv("components.helmfile.helm_aws_profile_pattern", "ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN")
	config.DefaultConfigHandler.BindEnv("components.helmfile.cluster_name_pattern", "ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME_PATTERN")

}

func helmfileRun(cmd *cobra.Command, commandName string, args []string) {
	handleHelpRequest(cmd, args)
	diffArgs := []string{commandName}
	diffArgs = append(diffArgs, args...)
	info := getConfigAndStacksInfo("helmfile", cmd, diffArgs)
	err := e.ExecuteHelmfile(info)
	if err != nil {
		u.PrintErrorMarkdownAndExit("", err, "")
	}
}
