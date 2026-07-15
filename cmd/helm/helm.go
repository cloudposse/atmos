package helm

import (
	"context"
	"strconv"
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
	helmCmd.AddCommand(newRepoCommand())

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
		"Choose a Helm component",
		componentArgCompletion,
		func(_ *flags.ParsedConfig) bool { return !hasSelectionFlags(cmd) },
	))
	parser = flags.NewStandardParser(options...)
	argsBuilder := flags.NewPositionalArgsBuilder()
	argsBuilder.AddArg(&flags.PositionalArgSpec{
		Name:           "component",
		Description:    "Helm component",
		Required:       true,
		TargetField:    "Component",
		CompletionFunc: componentArgCompletion,
		PromptTitle:    "Choose a Helm component",
	})
	specs, _, usage := argsBuilder.Build()
	parser.SetPositionalArgs(specs, validateOperationArgs, usage)
	parser.RegisterFlags(cmd)
	cmd.ValidArgsFunction = componentArgCompletion

	return cmd
}

// operationFlagOptions returns the standard-parser options for a Helm operation
// command: the shared selection/affected flags plus operation-specific output and target flags.
func operationFlagOptions(name string) []flags.Option {
	options := []flags.Option{
		flags.WithBoolFlag("all", "", false, "Process all Helm components in dependency order."),
		flags.WithBoolFlag("affected", "", false, "Process affected Helm components in dependency order."),
		flags.WithBoolFlag("ci", "", false, "Enable CI mode for automated pipelines (writes job summary)."),
		flags.WithBoolFlag("include-dependents", "", false, "Include dependent components when processing affected Helm components."),
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

	if name == "template" {
		options = append(
			options,
			flags.WithStringFlag("output", "", "", "Write rendered manifests to a single multi-document YAML file."),
			flags.WithStringFlag("output-dir", "", "", "Write rendered manifests to a directory."),
			flags.WithBoolFlag("split", "", false, "Write one rendered manifest file per object. Requires --output-dir."),
		)
	}

	if name == "apply" || name == "deploy" {
		options = append(
			options,
			flags.WithStringFlag("target", "", "", "Provision target to deliver to (e.g. a git deployment repository). Defaults to provision.default, otherwise the cluster."),
		)
	}

	if name == "diff" || name == "plan" {
		options = append(
			options,
			flags.WithStringFlag("against", "", "", "Baseline to diff against: 'release' (the deployed release, default), or 'target[:<name>]' for the git deployment-repo provision target (offline)."),
			flags.WithStringFlag("from-manifest", "", "", "Diff against a local baseline manifest file instead of the cluster (offline)."),
			flags.WithIntFlag("context", "", 3, "Number of unchanged context lines to show around each change."),
		)
	}

	return options
}

func validateOperationArgs(cmd *cobra.Command, args []string) error {
	all, _ := cmd.Flags().GetBool(flagAll)
	affected, _ := cmd.Flags().GetBool(flagAffected)
	if all && affected {
		return errUtils.ErrHelmFlagsMutuallyExclusive
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
		return errUtils.ErrHelmComponentArgRequired
	}
	return nil
}

func hasSelectionFlags(cmd *cobra.Command) bool {
	all, _ := cmd.Flags().GetBool(flagAll)
	affected, _ := cmd.Flags().GetBool(flagAffected)
	tagsFlag, _ := cmd.Flags().GetStringSlice("tags")
	labelsFlag, _ := cmd.Flags().GetString("labels")
	return all || affected || len(tagsFlag) > 0 || labelsFlag != ""
}

// componentArgCompletion returns names for native Helm components, optionally
// limited to the selected stack.
func componentArgCompletion(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	info := buildConfigAndStacksInfo(cmd)
	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	stacksMap, err := e.ExecuteDescribeStacks(&atmosConfig, info.Stack, nil, []string{cfg.HelmComponentType}, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	components, err := component.ListAllComponents(context.Background(), cfg.HelmComponentType, stacksMap)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return components, cobra.ShellCompDirectiveNoFileComp
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
	for _, name := range []string{"repo-path", "base", "ref", "sha", "ssh-key", "ssh-key-password", "target", "against", "from-manifest"} {
		if flag := cmd.Flag(name); flag != nil {
			result[name] = flag.Value.String()
		}
	}
	if contextFlag := cmd.Flag("context"); contextFlag != nil {
		if n, err := strconv.Atoi(contextFlag.Value.String()); err == nil {
			result["context"] = n
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
	info.ComponentType = cfg.HelmComponentType
	info.SubCommand = subCommand
	info.CliArgs = []string{cfg.HelmComponentType, subCommand}
	if len(args) > 0 {
		info.ComponentFromArg = args[0]
	}
	return info
}
