// Package cloudformation implements the `atmos aws cloudformation` (alias `cfn`)
// command group for the native aws/cloudformation component type.
package cloudformation

import (
	"context"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/aws/cloudformation/backend"
	"github.com/cloudposse/atmos/cmd/aws/cloudformation/source"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
)

const (
	flagAll      = "all"
	flagAffected = "affected"
	flagTags     = "tags"
	flagLabels   = "labels"
	// The valueTrue const is the string representation of a set boolean flag.
	valueTrue = "true"

	flagAutoApprove = "auto-approve"

	// The msgSkipConfirmation const is the shared --auto-approve flag description
	// across every mutating operation.
	msgSkipConfirmation = "Skip interactive confirmation."

	// The subCommandApply/subCommandDelete consts are the Operation-dispatch
	// identifiers shared by the top-level apply/deploy and delete verbs, and by
	// verb-group entries that reuse the same literal (e.g. `changeset delete`'s
	// use name).
	subCommandApply  = "apply"
	subCommandDelete = "delete"
)

var cloudFormationParser *flags.StandardParser

var (
	cfnInitCliConfig     = cfg.InitCliConfig
	cfnDescribeStacks    = e.ExecuteDescribeStacks
	cfnListAllComponents = component.ListAllComponents
)

// CloudFormationCmd is the `atmos aws cloudformation` command group, mounted
// under `atmos aws` alongside ecr/eks/security/compliance. The nested
// Annotations-based experimental gate mirrors cmd/terraform/backend/backend.go:
// no registry-level IsExperimental() change is needed, and `aws` itself (and
// its other subcommands) stay stable.
var CloudFormationCmd = &cobra.Command{
	Use:     "cloudformation",
	Aliases: []string{"cfn"},
	Short:   "Manage native aws/cloudformation components",
	Long:    "Deploy, inspect, and delete native aws/cloudformation stacks using the AWS SDK for Go v2. No external binary dependency.",
	Annotations: map[string]string{
		"experimental": "true",
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Usage()
	},
}

func init() {
	cloudFormationParser = flags.NewStandardParser(flags.WithCommonFlags())
	cloudFormationParser.RegisterPersistentFlags(CloudFormationCmd)

	if err := cloudFormationParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	CloudFormationCmd.AddCommand(newOperationCommand("render", "render", "Render the local template client-side (no API calls)"))
	CloudFormationCmd.AddCommand(newOperationCommand("plan", "diff", "Preview changes an apply would make"))
	CloudFormationCmd.AddCommand(newOperationCommand("diff", "diff", "Show changes an apply would make"))
	CloudFormationCmd.AddCommand(newOperationCommand(subCommandApply, subCommandApply, "Create or update the stack"))
	CloudFormationCmd.AddCommand(newOperationCommand("deploy", subCommandApply, "Apply with --auto-approve"))
	CloudFormationCmd.AddCommand(newOperationCommand(subCommandDelete, subCommandDelete, "Delete the stack"))
	CloudFormationCmd.AddCommand(newOperationCommand("validate", "validate", "Validate the template server-side"))
	outputCmd := newOperationCommand("output", "output", "Show the deployed stack's Outputs")
	outputCmd.Aliases = []string{"outputs"}
	CloudFormationCmd.AddCommand(outputCmd)
	CloudFormationCmd.AddCommand(newChangesetCmd())
	CloudFormationCmd.AddCommand(newDriftCmd())
	CloudFormationCmd.AddCommand(newGetCmd())
	CloudFormationCmd.AddCommand(newStackSetCmd())
	CloudFormationCmd.AddCommand(newOperationCommand("fmt", "fmt", "Format the local template in place (or check formatting with --check)"))
	CloudFormationCmd.AddCommand(newOperationCommand("tree", "tree", "Render the nested-stack dependency tree"))
	CloudFormationCmd.AddCommand(newOperationCommand("logs", "logs", "Show the combined event log across a stack and its nested stacks"))
	CloudFormationCmd.AddCommand(newOperationCommand("watch", "watch", "Attach to a stack's in-progress (or already-terminal) operation and stream events"))
	CloudFormationCmd.AddCommand(newListCmd())
	CloudFormationCmd.AddCommand(source.GetSourceCommand())
	CloudFormationCmd.AddCommand(backend.GetBackendCommand())
}

// newChangesetCmd is the `atmos aws cloudformation changeset` verb group: manual
// control over changesets, complementing apply/deploy/diff's implicit flow.
func newChangesetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "changeset",
		Short: "Manage aws/cloudformation changesets directly",
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Usage() },
	}
	cmd.AddCommand(newOperationCommand("create", "changeset-create", "Create a changeset and leave it for later review/execution"))
	cmd.AddCommand(newOperationCommand("execute", "changeset-execute", "Execute a previously-created, named changeset"))
	cmd.AddCommand(newOperationCommand("list", "changeset-list", "List a stack's changesets"))
	cmd.AddCommand(newOperationCommand(subCommandDelete, "changeset-delete", "Delete a named changeset"))
	return cmd
}

// newDriftCmd is the `atmos aws cloudformation drift` verb group.
func newDriftCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Detect and describe drift against a deployed stack",
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Usage() },
	}
	cmd.AddCommand(newOperationCommand("detect", "drift-detect", "Run drift detection against the deployed stack"))
	cmd.AddCommand(newOperationCommand("describe", "drift-describe", "Show the results of the most recent drift detection"))
	return cmd
}

// newGetCmd is the `atmos aws cloudformation get` verb group.
func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Fetch a deployed stack's template or policy",
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Usage() },
	}
	cmd.AddCommand(newOperationCommand("template", "get-template", "Fetch the deployed stack's template"))
	cmd.AddCommand(newOperationCommand("policy", "get-policy", "Fetch the deployed stack's policy"))
	return cmd
}

// newStackSetCmd is the `atmos aws cloudformation stackset` verb group:
// multi-account/multi-region deployment orchestration via a `kind:
// aws/stackset` provision target.
func newStackSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stackset",
		Short: "Manage multi-account/multi-region StackSets",
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Usage() },
	}
	cmd.AddCommand(newOperationCommand("create", "stackset-create", "Create a StackSet (and its initial stack instances, if configured)"))
	cmd.AddCommand(newOperationCommand("update", "stackset-update", "Update a StackSet's template/parameters/capabilities"))
	cmd.AddCommand(newOperationCommand(subCommandDelete, "stackset-delete", "Delete every stack instance, then the StackSet itself"))
	cmd.AddCommand(newOperationCommand("instances", "stackset-instances", "List a StackSet's stack instances"))
	return cmd
}

// newOperationCommand builds an `atmos aws cloudformation <use> [component]`
// command (optionally nested under a verb group, e.g. `changeset create`).
// Use is the cobra command name (what the user types); subCommand is the
// internal Operation-dispatch identifier passed to provider.Execute (e.g.
// "changeset-create") and to operationFlagOptions/getOperationFlags — they
// differ for grouped verbs, where the same subCommand can be reached under a
// friendlier use (e.g. "plan" and "diff" both dispatch as "diff").
func newOperationCommand(use, subCommand, short string) *cobra.Command {
	var parser *flags.StandardParser
	cmd := &cobra.Command{
		Use:   use + " [component]",
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			parsed, err := parser.Parse(context.Background(), args)
			if err != nil {
				return err
			}
			return runOperation(cmd, subCommand, parsed.GetPositionalArgs())
		},
	}

	options := operationFlagOptions(use, subCommand)
	options = append(options, flags.WithConditionalPositionalArgPrompt(
		"component",
		"Choose an aws/cloudformation component",
		componentArgCompletion,
		func(_ *flags.ParsedConfig) bool { return !hasSelectionFlags(cmd) },
	))
	parser = flags.NewStandardParser(options...)
	argsBuilder := flags.NewPositionalArgsBuilder()
	argsBuilder.AddArg(&flags.PositionalArgSpec{
		Name:           "component",
		Description:    "aws/cloudformation component",
		Required:       true,
		TargetField:    "Component",
		CompletionFunc: componentArgCompletion,
		PromptTitle:    "Choose an aws/cloudformation component",
	})
	specs, _, usage := argsBuilder.Build()
	parser.SetPositionalArgs(specs, validateOperationArgs, usage)
	parser.RegisterFlags(cmd)
	cmd.ValidArgsFunction = componentArgCompletion

	return cmd
}

// operationFlagOptions returns the standard-parser options for an
// aws/cloudformation operation command: the shared selection/affected flags
// plus operation-specific flags. Use is the cobra command name (only needed to
// distinguish apply's and deploy's differing --auto-approve default);
// subCommand is the unique Operation-dispatch identifier every other switch
// below keys on (use alone collides across verb groups, e.g. top-level
// `delete` and `changeset delete`).
func operationFlagOptions(use, subCommand string) []flags.Option {
	options := []flags.Option{
		flags.WithBoolFlag(flagAll, "", false, "Process all aws/cloudformation components in dependency order."),
		flags.WithBoolFlag(flagAffected, "", false, "Process affected aws/cloudformation components in dependency order."),
		flags.WithBoolFlag("include-dependents", "", false, "Include dependent components when processing affected aws/cloudformation components."),
		flags.WithStringFlag("repo-path", "", "", "Path to the already cloned target repository to use as the affected baseline."),
		flags.WithStringFlag("base", "", "", "Git base ref or SHA to compare against for affected detection."),
		flags.WithStringFlag("ref", "", "", "Git ref to compare against for affected detection."),
		flags.WithStringFlag("sha", "", "", "Git SHA to compare against for affected detection."),
		flags.WithStringFlag("ssh-key", "", "", "Path to the SSH private key used to clone the target ref for affected detection."),
		flags.WithStringFlag("ssh-key-password", "", "", "Password for the SSH private key used to clone the target ref for affected detection."),
		flags.WithBoolFlag("clone-target-ref", "", false, "Clone the target ref instead of checking it out in the current repository for affected detection."),
		flags.WithStringSliceFlag(flagTags, "", nil, "Filter by tags (comma-separated, matches any): --tags=production,tier-1"),
		flags.WithStringFlag(flagLabels, "", "", "Filter by labels (comma-separated key=value or key:value pairs, matches all): --labels=cost-center=platform,compliance=sox"),
	}
	options = append(options, operationSpecificFlagOptions(use, subCommand)...)
	return options
}

// operationSpecificFlagOptions returns the flags specific to one operation,
// split out of operationFlagOptions to keep its cyclomatic complexity low.
func operationSpecificFlagOptions(use, subCommand string) []flags.Option {
	switch subCommand {
	case subCommandApply:
		options := []flags.Option{
			flags.WithBoolFlag(flagAutoApprove, "", use == "deploy", msgSkipConfirmation),
			flags.WithStringFlag("target", "", "", "Provision target to deliver to. Defaults to provision.default, otherwise the implicit direct-deploy target."),
		}
		return options
	case subCommandDelete:
		return []flags.Option{
			flags.WithBoolFlag(flagAutoApprove, "", false, msgSkipConfirmation),
			flags.WithStringSliceFlag("retain-resources", "", nil, "Logical IDs of resources to retain (only valid for a DELETE_FAILED stack)."),
			flags.WithBoolFlag("disable-termination-protection", "", false, "Disable termination protection before deleting (never done silently)."),
		}
	case "output":
		return []flags.Option{
			flags.WithStringFlag("format", "", "table", "Output format: json|yaml|hcl|env|dotenv|bash|csv|tsv|table|github."),
			flags.WithBoolFlag("flatten", "", false, "Flatten nested Outputs into compound keys."),
			flags.WithBoolFlag("uppercase", "", false, "Uppercase Output keys."),
		}
	case "changeset-execute":
		return []flags.Option{
			flags.WithRequiredStringFlag("changeset-name", "", "Name of the changeset to execute."),
			flags.WithBoolFlag(flagAutoApprove, "", false, msgSkipConfirmation),
		}
	case "changeset-delete":
		return []flags.Option{
			flags.WithRequiredStringFlag("changeset-name", "", "Name of the changeset to delete."),
		}
	case "drift-detect":
		return []flags.Option{
			flags.WithBoolFlag("fail-on-drift", "", false, "Exit non-zero if drift is detected (for CI)."),
		}
	case "get-template":
		return []flags.Option{
			flags.WithBoolFlag("original", "", false, "Fetch the user-submitted template instead of the processed one."),
		}
	case "fmt":
		return []flags.Option{
			flags.WithBoolFlag("check", "", false, "Report whether the template is formatted without writing changes (non-zero exit if not)."),
		}
	default:
		return phase3FlagOptions(subCommand)
	}
}

// phase3FlagOptions returns flags specific to stackset/observability
// operations, split out of operationSpecificFlagOptions to keep its
// cyclomatic complexity low.
func phase3FlagOptions(subCommand string) []flags.Option {
	switch subCommand {
	case "stackset-create", "stackset-update":
		return []flags.Option{
			flags.WithBoolFlag(flagAutoApprove, "", false, msgSkipConfirmation),
			flags.WithStringFlag("target", "", "", "The `kind: aws/stackset` provision target to use. Required when more than one is declared."),
		}
	case "stackset-delete":
		return []flags.Option{
			flags.WithBoolFlag(flagAutoApprove, "", false, msgSkipConfirmation),
		}
	case "logs":
		return []flags.Option{
			flags.WithBoolFlag("chart", "", false, "Render a per-resource timeline instead of a flat chronological event list."),
		}
	default:
		return nil
	}
}

func validateOperationArgs(cmd *cobra.Command, args []string) error {
	all, _ := cmd.Flags().GetBool(flagAll)
	affected, _ := cmd.Flags().GetBool(flagAffected)
	if all && affected {
		return errUtils.ErrAwsCloudFormationFlagsMutuallyExclusive
	}

	tagsFlag, _ := cmd.Flags().GetStringSlice(flagTags)
	labelsFlag, _ := cmd.Flags().GetString(flagLabels)
	if _, err := tags.ParseLabelsFlag(labelsFlag); err != nil {
		return err
	}
	hasTagsOrLabels := len(tagsFlag) > 0 || labelsFlag != ""

	if all || affected || hasTagsOrLabels {
		return validateSelectionFlags(args)
	}
	if len(args) != 1 {
		return errUtils.ErrAwsCloudFormationComponentArgRequired
	}
	return nil
}

func hasSelectionFlags(cmd *cobra.Command) bool {
	all, _ := cmd.Flags().GetBool(flagAll)
	affected, _ := cmd.Flags().GetBool(flagAffected)
	tagsFlag, _ := cmd.Flags().GetStringSlice(flagTags)
	labelsFlag, _ := cmd.Flags().GetString(flagLabels)
	return all || affected || len(tagsFlag) > 0 || labelsFlag != ""
}

func validateSelectionFlags(args []string) error {
	if len(args) != 0 {
		return errUtils.ErrAwsCloudFormationComponentArgWithSelection
	}
	return nil
}

// componentArgCompletion returns names for native aws/cloudformation
// components, optionally limited to the selected stack.
func componentArgCompletion(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	info := buildConfigAndStacksInfo(cmd)
	atmosConfig, err := cfnInitCliConfig(info, true)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	stacksMap, err := cfnDescribeStacks(&atmosConfig, info.Stack, nil, []string{cfg.CloudFormationComponentType}, nil, false, false, false, false, nil, nil)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	components, err := cfnListAllComponents(context.Background(), cfg.CloudFormationComponentType, stacksMap)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return components, cobra.ShellCompDirectiveNoFileComp
}

func runOperation(cmd *cobra.Command, subCommand string, args []string) error {
	info := initConfigAndStacksInfo(cmd, subCommand, args)
	provider := component.MustGetProvider(cfg.CloudFormationComponentType)

	return provider.Execute(&component.ExecutionContext{
		ComponentType:       cfg.CloudFormationComponentType,
		Component:           info.ComponentFromArg,
		Stack:               info.Stack,
		Command:             cfg.CloudFormationComponentType,
		SubCommand:          subCommand,
		ConfigAndStacksInfo: info,
		Args:                args,
		Flags:               getOperationFlags(cmd),
	})
}

func getOperationFlags(cmd *cobra.Command) map[string]any {
	result := make(map[string]any)
	for _, name := range []string{flagAll, flagAffected, "include-dependents", "clone-target-ref", flagAutoApprove, "disable-termination-protection", "flatten", "uppercase", "fail-on-drift", "original", "check", "chart"} {
		if flag := cmd.Flag(name); flag != nil {
			result[name] = flag.Value.String() == valueTrue
		}
	}
	for _, name := range []string{"repo-path", "base", "ref", "sha", "ssh-key", "ssh-key-password", "target", "format", "changeset-name"} {
		if flag := cmd.Flag(name); flag != nil {
			result[name] = flag.Value.String()
		}
	}
	if retainFlag, err := cmd.Flags().GetStringSlice("retain-resources"); err == nil && len(retainFlag) > 0 {
		result["retain-resources"] = retainFlag
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

	applySelectionFlags(cmd, &info)
	return info
}

// applySelectionFlags applies the stack/dry-run/all/affected/tags/labels flags
// onto info, split out of buildConfigAndStacksInfo to keep its cyclomatic
// complexity low.
func applySelectionFlags(cmd *cobra.Command, info *schema.ConfigAndStacksInfo) {
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
	applyTagsAndLabelsFlags(cmd, info)
}

// applyTagsAndLabelsFlags applies the tags/labels selection flags onto info.
func applyTagsAndLabelsFlags(cmd *cobra.Command, info *schema.ConfigAndStacksInfo) {
	if tagsSlice, err := cmd.Flags().GetStringSlice(flagTags); err == nil {
		info.Tags = tags.ParseTagsFlag(strings.Join(tagsSlice, ","))
	}
	if labelsFlag := cmd.Flag(flagLabels); labelsFlag != nil {
		// Error ignored: validateOperationArgs already rejected malformed --labels before RunE.
		info.Labels, _ = tags.ParseLabelsFlag(labelsFlag.Value.String())
	}
}

func initConfigAndStacksInfo(cmd *cobra.Command, subCommand string, args []string) schema.ConfigAndStacksInfo {
	info := buildConfigAndStacksInfo(cmd)
	info.ComponentType = cfg.CloudFormationComponentType
	info.SubCommand = subCommand
	info.CliArgs = []string{cfg.CloudFormationComponentType, subCommand}
	if len(args) > 0 {
		info.ComponentFromArg = args[0]
	}
	return info
}
