package kubernetes

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
)

const (
	flagOutput   = "output"
	flagAll      = "all"
	flagAffected = "affected"
	// The valueTrue const is the string representation of a set boolean flag.
	valueTrue = "true"
)

var kubernetesParser *flags.StandardParser

var (
	kubernetesInitCliConfig     = cfg.InitCliConfig
	kubernetesDescribeStacks    = e.ExecuteDescribeStacks
	kubernetesListAllComponents = component.ListAllComponents
)

var kubernetesCmd = &cobra.Command{
	Use:     "kubernetes",
	Aliases: []string{"k8s"},
	Short:   "Manage Kubernetes-native components",
	Long:    "Render, validate, diff, apply, and delete Kubernetes-native Atmos components using Kubernetes Go SDKs.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

func init() {
	kubernetesParser = flags.NewStandardParser(flags.WithCommonFlags())
	kubernetesParser.RegisterPersistentFlags(kubernetesCmd)

	if err := kubernetesParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	kubernetesCmd.AddCommand(newOperationCommand("render", "Render Kubernetes manifests"))
	kubernetesCmd.AddCommand(newOperationCommand("diff", "Show server-side Kubernetes manifest changes"))
	kubernetesCmd.AddCommand(newOperationCommand("plan", "Preview server-side Kubernetes manifest changes"))
	kubernetesCmd.AddCommand(newOperationCommand("apply", "Apply Kubernetes manifests"))
	kubernetesCmd.AddCommand(newOperationCommand("deploy", "Deploy Kubernetes manifests"))
	kubernetesCmd.AddCommand(newOperationCommand("delete", "Delete Kubernetes manifests"))
	kubernetesCmd.AddCommand(newOperationCommand("validate", "Validate rendered Kubernetes manifests"))

	internal.Register(&CommandProvider{})
}

type CommandProvider struct{}

func (p *CommandProvider) GetCommand() *cobra.Command {
	return kubernetesCmd
}

func (p *CommandProvider) GetName() string {
	return "kubernetes"
}

func (p *CommandProvider) GetGroup() string {
	return "Core Stack Commands"
}

func (p *CommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

func (p *CommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

func (p *CommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (p *CommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

func (p *CommandProvider) IsExperimental() bool {
	return true
}

func newOperationCommand(name string, short string) *cobra.Command {
	var parser *flags.StandardParser
	cmd := &cobra.Command{
		Use:   name + " [component]",
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			parsed, err := parser.Parse(context.Background(), args)
			if err != nil {
				return err
			}
			return runOperation(cmd, name, parsed.GetPositionalArgs())
		},
	}

	options := operationFlagOptions(name)
	options = append(options, flags.WithConditionalPositionalArgPrompt(
		"component",
		"Choose a Kubernetes component",
		componentArgCompletion,
		func(_ *flags.ParsedConfig) bool { return !hasSelectionFlags(cmd) },
	))
	parser = flags.NewStandardParser(options...)
	argsBuilder := flags.NewPositionalArgsBuilder()
	argsBuilder.AddArg(&flags.PositionalArgSpec{
		Name:           "component",
		Description:    "Kubernetes component",
		Required:       true,
		TargetField:    "Component",
		CompletionFunc: componentArgCompletion,
		PromptTitle:    "Choose a Kubernetes component",
	})
	specs, _, usage := argsBuilder.Build()
	parser.SetPositionalArgs(specs, validateOperationArgs, usage)
	parser.RegisterFlags(cmd)
	cmd.ValidArgsFunction = componentArgCompletion

	return cmd
}

// operationFlagOptions returns the standard-parser options for a Kubernetes operation
// command: the shared selection/affected flags plus operation-specific output and target flags.
func operationFlagOptions(name string) []flags.Option {
	options := []flags.Option{
		flags.WithBoolFlag("all", "", false, "Process all Kubernetes components in dependency order."),
		flags.WithBoolFlag("affected", "", false, "Process affected Kubernetes components in dependency order."),
		flags.WithBoolFlag("ci", "", false, "Enable CI mode for automated pipelines (writes job summary)."),
		flags.WithBoolFlag("include-dependents", "", false, "Include dependent components when processing affected Kubernetes components."),
		flags.WithStringFlag("repo-path", "", "", "Path to the already cloned target repository to use as the affected baseline."),
		flags.WithStringFlag("base", "", "", "Git base ref or SHA to compare against for affected detection."),
		flags.WithStringFlag("ref", "", "", "Git ref to compare against for affected detection."),
		flags.WithStringFlag("sha", "", "", "Git SHA to compare against for affected detection."),
		flags.WithStringFlag("ssh-key", "", "", "Path to the SSH private key used to clone the target ref for affected detection."),
		flags.WithStringFlag("ssh-key-password", "", "", "Password for the SSH private key used to clone the target ref for affected detection."),
		flags.WithBoolFlag("clone-target-ref", "", false, "Clone the target ref instead of checking it out in the current repository for affected detection."),
		flags.WithStringSliceFlag("tags", "", nil, "Filter by tags (comma-separated, matches any): --tags=production,tier-1"),
		flags.WithStringFlag("labels", "", "", "Filter by labels (comma-separated key=value pairs, matches all): --labels=cost-center=platform,compliance=sox"),
	}

	if name == "render" {
		options = append(
			options,
			flags.WithStringFlag("output", "", "", "Write rendered manifests to a single multi-document YAML file."),
			flags.WithStringFlag("output-dir", "", "", "Write rendered manifests to a directory."),
			flags.WithBoolFlag("split", "", false, "Write one rendered manifest file per Kubernetes object. Requires --output-dir."),
		)
	}

	if name == "apply" || name == "deploy" {
		options = append(
			options,
			flags.WithStringFlag("target", "", "", "Provision target to deliver to (e.g. a git deployment repository). Defaults to provision.default, otherwise the cluster."),
		)
	}

	if name == "validate" {
		options = append(
			options,
			flags.WithBoolFlag("server", "", false, "Also validate rendered manifests against the live cluster using a server-side dry-run apply. Requires a reachable cluster and kubeconfig."),
		)
	}

	return options
}

func hasSelectionFlags(cmd *cobra.Command) bool {
	all, _ := cmd.Flags().GetBool(flagAll)
	affected, _ := cmd.Flags().GetBool(flagAffected)
	tagsFlag, _ := cmd.Flags().GetStringSlice("tags")
	labelsFlag, _ := cmd.Flags().GetString("labels")
	return all || affected || len(tagsFlag) > 0 || labelsFlag != ""
}

// componentArgCompletion returns names for native Kubernetes components,
// optionally limited to the selected stack.
func componentArgCompletion(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	info := buildConfigAndStacksInfo(cmd)
	atmosConfig, err := kubernetesInitCliConfig(info, true)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	stacksMap, err := kubernetesDescribeStacks(&atmosConfig, info.Stack, nil, []string{cfg.KubernetesComponentType}, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	components, err := kubernetesListAllComponents(context.Background(), cfg.KubernetesComponentType, stacksMap)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return components, cobra.ShellCompDirectiveNoFileComp
}

func validateOperationArgs(cmd *cobra.Command, args []string) error {
	all, _ := cmd.Flags().GetBool(flagAll)
	affected, _ := cmd.Flags().GetBool(flagAffected)
	if all && affected {
		return errUtils.ErrKubernetesFlagsMutuallyExclusive
	}

	tagsFlag, _ := cmd.Flags().GetStringSlice("tags")
	labelsFlag, _ := cmd.Flags().GetString("labels")
	if _, err := tags.ParseLabelsFlag(labelsFlag); err != nil {
		return err
	}
	hasTagsOrLabels := len(tagsFlag) > 0 || labelsFlag != ""

	if all || affected || hasTagsOrLabels {
		return validateSelectionFlags(cmd, args)
	}
	if len(args) != 1 {
		return errUtils.ErrKubernetesComponentArgRequired
	}
	return nil
}

// validateSelectionFlags validates argument and output flag combinations when
// --all or --affected is set.
func validateSelectionFlags(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		return errUtils.ErrKubernetesComponentArgWithSelection
	}
	if cmd.Name() == "render" {
		output, _ := cmd.Flags().GetString(flagOutput)
		outputDir, _ := cmd.Flags().GetString("output-dir")
		if output != "" || outputDir != "" {
			return errUtils.ErrKubernetesOutputSingleComponentOnly
		}
	}
	return nil
}

func runOperation(cmd *cobra.Command, subCommand string, args []string) error {
	info := initConfigAndStacksInfo(cmd, subCommand, args)
	provider := component.MustGetProvider(cfg.KubernetesComponentType)

	return provider.Execute(&component.ExecutionContext{
		ComponentType:       cfg.KubernetesComponentType,
		Component:           info.ComponentFromArg,
		Stack:               info.Stack,
		Command:             cfg.KubernetesComponentType,
		SubCommand:          subCommand,
		ConfigAndStacksInfo: info,
		Args:                args,
		Flags:               getOperationFlags(cmd),
	})
}

func getOperationFlags(cmd *cobra.Command) map[string]any {
	result := make(map[string]any)
	for _, name := range []string{"all", "affected", "ci", "include-dependents", "clone-target-ref", "server"} {
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
	if tagsSlice, err := cmd.Flags().GetStringSlice("tags"); err == nil {
		info.Tags = tags.ParseTagsFlag(strings.Join(tagsSlice, ","))
	}
	if labelsFlag := cmd.Flag("labels"); labelsFlag != nil {
		// Error ignored: validateOperationArgs already rejected malformed --labels before RunE.
		info.Labels, _ = tags.ParseLabelsFlag(labelsFlag.Value.String())
	}

	return info
}

func initConfigAndStacksInfo(cmd *cobra.Command, subCommand string, args []string) schema.ConfigAndStacksInfo {
	info := buildConfigAndStacksInfo(cmd)
	info.ComponentType = cfg.KubernetesComponentType
	info.SubCommand = subCommand
	info.CliArgs = []string{cfg.KubernetesComponentType, subCommand}
	if len(args) > 0 {
		info.ComponentFromArg = args[0]
	}
	return info
}
