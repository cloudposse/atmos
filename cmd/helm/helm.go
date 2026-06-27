package helm

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	flagOutput   = "output"
	flagAll      = "all"
	flagAffected = "affected"
	// The valueTrue const is the string representation of a set boolean flag.
	valueTrue = "true"
)

var helmParser *flags.StandardParser

var helmCmd = &cobra.Command{
	Use:   "helm",
	Short: "Manage native Helm components",
	Long:  "Render, diff, apply, and delete native Helm Atmos components using the Helm Go SDK. Supports local, remote-repository, and OCI charts.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Usage()
	},
}

func init() {
	helmParser = flags.NewStandardParser(flags.WithCommonFlags())
	helmParser.RegisterPersistentFlags(helmCmd)

	if err := helmParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	helmCmd.AddCommand(newOperationCommand("template", "Render Helm chart manifests"))
	helmCmd.AddCommand(newOperationCommand("diff", "Show changes a Helm apply would make"))
	helmCmd.AddCommand(newOperationCommand("plan", "Preview changes a Helm apply would make"))
	helmCmd.AddCommand(newOperationCommand("apply", "Install or upgrade a Helm release"))
	helmCmd.AddCommand(newOperationCommand("deploy", "Deploy a Helm release"))
	helmCmd.AddCommand(newOperationCommand("delete", "Uninstall a Helm release"))

	internal.Register(&CommandProvider{})
}

// CommandProvider registers the helm command with the command registry.
type CommandProvider struct{}

func (p *CommandProvider) GetCommand() *cobra.Command { return helmCmd }

func (p *CommandProvider) GetName() string { return "helm" }

func (p *CommandProvider) GetGroup() string { return "Core Stack Commands" }

func (p *CommandProvider) GetAliases() []internal.CommandAlias { return nil }

func (p *CommandProvider) GetFlagsBuilder() flags.Builder { return nil }

func (p *CommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder { return nil }

func (p *CommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag { return nil }

func (p *CommandProvider) IsExperimental() bool { return true }

func newOperationCommand(name, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   name + " [component]",
		Args:  validateOperationArgs,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOperation(cmd, name, args)
		},
	}

	cmd.Flags().Bool("all", false, "Process all Helm components in dependency order.")
	cmd.Flags().Bool("affected", false, "Process affected Helm components in dependency order.")
	cmd.Flags().Bool("ci", false, "Enable CI mode for automated pipelines (writes job summary).")
	cmd.Flags().Bool("include-dependents", false, "Include dependent components when processing affected Helm components.")
	cmd.Flags().String("repo-path", "", "Path to the already cloned target repository to use as the affected baseline.")
	cmd.Flags().String("base", "", "Git base ref or SHA to compare against for affected detection.")
	cmd.Flags().String("ref", "", "Git ref to compare against for affected detection.")
	cmd.Flags().String("sha", "", "Git SHA to compare against for affected detection.")
	cmd.Flags().String("ssh-key", "", "Path to the SSH private key used to clone the target ref for affected detection.")
	cmd.Flags().String("ssh-key-password", "", "Password for the SSH private key used to clone the target ref for affected detection.")
	cmd.Flags().Bool("clone-target-ref", false, "Clone the target ref instead of checking it out in the current repository for affected detection.")

	if name == "template" {
		cmd.Flags().String("output", "", "Write rendered manifests to a single multi-document YAML file.")
		cmd.Flags().String("output-dir", "", "Write rendered manifests to a directory.")
		cmd.Flags().Bool("split", false, "Write one rendered manifest file per object. Requires --output-dir.")
	}

	if name == "apply" || name == "deploy" {
		cmd.Flags().String("target", "", "Provision target to deliver to (e.g. a git deployment repository). Defaults to provision.default, otherwise the cluster.")
	}

	return cmd
}

func validateOperationArgs(cmd *cobra.Command, args []string) error {
	all, _ := cmd.Flags().GetBool(flagAll)
	affected, _ := cmd.Flags().GetBool(flagAffected)
	if all && affected {
		return errUtils.ErrHelmFlagsMutuallyExclusive
	}
	if all || affected {
		return validateSelectionFlags(cmd, args)
	}
	if len(args) != 1 {
		return errUtils.ErrHelmComponentArgRequired
	}
	return nil
}

func validateSelectionFlags(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		return errUtils.ErrHelmComponentArgWithSelection
	}
	if cmd.Name() == "template" {
		output, _ := cmd.Flags().GetString(flagOutput)
		outputDir, _ := cmd.Flags().GetString("output-dir")
		if output != "" || outputDir != "" {
			return errUtils.ErrHelmOutputSingleComponentOnly
		}
	}
	return nil
}

func runOperation(cmd *cobra.Command, subCommand string, args []string) error {
	info := initConfigAndStacksInfo(cmd, subCommand, args)
	provider := component.MustGetProvider(cfg.HelmComponentType)

	return provider.Execute(&component.ExecutionContext{
		ComponentType:       cfg.HelmComponentType,
		Component:           info.ComponentFromArg,
		Stack:               info.Stack,
		Command:             cfg.HelmComponentType,
		SubCommand:          subCommand,
		ConfigAndStacksInfo: info,
		Args:                args,
		Flags:               getOperationFlags(cmd),
	})
}

func getOperationFlags(cmd *cobra.Command) map[string]any {
	result := make(map[string]any)
	for _, name := range []string{"all", "affected", "ci", "include-dependents", "clone-target-ref"} {
		if flag := cmd.Flag(name); flag != nil {
			result[name] = flag.Value.String() == valueTrue
		}
	}
	for _, name := range []string{"repo-path", "base", "ref", "sha", "ssh-key", "ssh-key-password", "target"} {
		if flag := cmd.Flag(name); flag != nil {
			result[name] = flag.Value.String()
		}
	}
	if outputFlag := cmd.Flag(flagOutput); outputFlag != nil {
		result[flagOutput] = outputFlag.Value.String()
	}
	if outputDirFlag := cmd.Flag("output-dir"); outputDirFlag != nil {
		result["output_dir"] = outputDirFlag.Value.String()
	}
	if splitFlag := cmd.Flag("split"); splitFlag != nil {
		result["split"] = splitFlag.Value.String() == valueTrue
	}
	return result
}

func buildConfigAndStacksInfo(cmd *cobra.Command) schema.ConfigAndStacksInfo {
	v := viper.GetViper()
	globalFlags := flags.ParseGlobalFlags(cmd, v)

	info := schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		Identity:                cfg.NormalizeIdentityValue(globalFlags.Identity.Value()),
		ProfilesFromArg:         globalFlags.Profile,
		ProcessTemplates:        true,
		ProcessFunctions:        true,
	}

	if stackFlag := cmd.Flag("stack"); stackFlag != nil && stackFlag.Value.String() != "" {
		info.Stack = stackFlag.Value.String()
	}
	if dryRunFlag := cmd.Flag("dry-run"); dryRunFlag != nil && dryRunFlag.Value.String() == valueTrue {
		info.DryRun = true
	}
	if allFlag := cmd.Flag(flagAll); allFlag != nil && allFlag.Value.String() == valueTrue {
		info.All = true
	}
	if affectedFlag := cmd.Flag(flagAffected); affectedFlag != nil && affectedFlag.Value.String() == valueTrue {
		info.Affected = true
	}

	return info
}

func initConfigAndStacksInfo(cmd *cobra.Command, subCommand string, args []string) schema.ConfigAndStacksInfo {
	info := buildConfigAndStacksInfo(cmd)
	info.ComponentType = cfg.HelmComponentType
	info.SubCommand = subCommand
	info.CliArgs = []string{cfg.HelmComponentType, subCommand}
	if len(args) > 0 {
		info.ComponentFromArg = args[0]
	}
	return info
}
