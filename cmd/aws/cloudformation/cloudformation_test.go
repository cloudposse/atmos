package cloudformation

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// recordingProvider is a minimal component.ComponentProvider that records the
// ExecutionContext it was called with, used to verify runOperation wires the
// context correctly without depending on the real AWS SDK-backed provider.
type recordingProvider struct {
	executed []*component.ExecutionContext
}

func (p *recordingProvider) GetType() string { return cfg.CloudFormationComponentType }

func (p *recordingProvider) GetGroup() string { return "test" }

func (p *recordingProvider) GetBasePath(*schema.AtmosConfiguration) string { return "" }

func (p *recordingProvider) ListComponents(context.Context, string, map[string]any) ([]string, error) {
	return nil, nil
}

func (p *recordingProvider) ValidateComponent(map[string]any) error { return nil }

func (p *recordingProvider) Execute(ctx *component.ExecutionContext) error {
	p.executed = append(p.executed, ctx)
	return nil
}

func (p *recordingProvider) GenerateArtifacts(*component.ExecutionContext) error { return nil }

func (p *recordingProvider) GetAvailableCommands() []string { return nil }

func configuredOperationCommand(t *testing.T, name string, values map[string]string) *cobra.Command {
	t.Helper()

	cmd := newOperationCommand(name, name, name)
	for flagName, value := range values {
		require.NoError(t, cmd.Flags().Set(flagName, value))
	}
	return cmd
}

func TestCloudFormationCmdAttributes(t *testing.T) {
	assert.Equal(t, "cloudformation", CloudFormationCmd.Use)
	assert.Equal(t, []string{"cfn"}, CloudFormationCmd.Aliases)
	assert.Equal(t, "true", CloudFormationCmd.Annotations["experimental"])
	assert.Contains(t, CloudFormationCmd.Short, "aws/cloudformation")

	var subcommands []string
	for _, c := range CloudFormationCmd.Commands() {
		subcommands = append(subcommands, c.Name())
	}
	assert.ElementsMatch(t, []string{
		"render", "plan", "diff", "apply", "deploy", "delete", "validate", "output",
		"changeset", "drift", "fmt", "get", "list", "source",
		"logs", "stackset", "tree", "watch",
	}, subcommands)

	// "output" registers the "outputs" alias.
	for _, c := range CloudFormationCmd.Commands() {
		if c.Name() == "output" {
			assert.Equal(t, []string{"outputs"}, c.Aliases)
		}
	}
}

func TestCloudFormationCmdRunEShowsUsage(t *testing.T) {
	require.NoError(t, CloudFormationCmd.RunE(CloudFormationCmd, nil))
}

func TestNewOperationCommandRegistersExpectedFlags(t *testing.T) {
	renderCmd := newOperationCommand("render", "render", "Render")
	for _, name := range []string{
		"all", "affected", "include-dependents", "repo-path", "base", "ref", "sha",
		"ssh-key", "ssh-key-password", "clone-target-ref", "tags", "labels",
	} {
		assert.NotNil(t, renderCmd.Flag(name), "expected render flag %q", name)
	}
	assert.Nil(t, renderCmd.Flag("auto-approve"))
	assert.Nil(t, renderCmd.Flag("target"))
	assert.Nil(t, renderCmd.Flag("retain-resources"))
	assert.Nil(t, renderCmd.Flag("format"))

	applyCmd := newOperationCommand("apply", "apply", "Apply")
	assert.NotNil(t, applyCmd.Flag("all"))
	assert.NotNil(t, applyCmd.Flag("auto-approve"))
	assert.Equal(t, "false", applyCmd.Flag("auto-approve").DefValue)
	assert.NotNil(t, applyCmd.Flag("target"))
	assert.NotNil(t, applyCmd.ValidArgsFunction)
	require.NoError(t, applyCmd.Args(applyCmd, nil), "the missing component must reach the interactive prompt flow")
	require.Error(t, applyCmd.Args(applyCmd, []string{"app", "extra"}))

	deployCmd := newOperationCommand("deploy", subCommandApply, "Deploy")
	require.NotNil(t, deployCmd.Flag("auto-approve"))
	assert.Equal(t, "true", deployCmd.Flag("auto-approve").DefValue, "deploy defaults --auto-approve to true")
	assert.NotNil(t, deployCmd.Flag("target"))

	deleteCmd := newOperationCommand("delete", "delete", "Delete")
	assert.NotNil(t, deleteCmd.Flag("auto-approve"))
	assert.NotNil(t, deleteCmd.Flag("retain-resources"))
	assert.NotNil(t, deleteCmd.Flag("disable-termination-protection"))
	assert.Nil(t, deleteCmd.Flag("target"), "--target is only for apply/deploy")

	outputCmd := newOperationCommand("output", "output", "Show output")
	require.NotNil(t, outputCmd.Flag("format"))
	assert.Equal(t, "table", outputCmd.Flag("format").DefValue)
	assert.NotNil(t, outputCmd.Flag("flatten"))
	assert.NotNil(t, outputCmd.Flag("uppercase"))
	assert.Nil(t, outputCmd.Flag("auto-approve"))
	assert.Nil(t, outputCmd.Flag("target"))
}

func TestSelectionFlagsAndComponentCompletion(t *testing.T) {
	for _, flag := range []string{"all", "affected", "tags", "labels"} {
		t.Run(flag, func(t *testing.T) {
			cmd := newOperationCommand("apply", "apply", "Apply")
			assert.False(t, hasSelectionFlags(cmd))
			if flag == "all" || flag == "affected" {
				require.NoError(t, cmd.Flags().Set(flag, "true"))
			} else {
				require.NoError(t, cmd.Flags().Set(flag, "value"))
			}
			assert.True(t, hasSelectionFlags(cmd))
		})
	}

	cmd := newOperationCommand("apply", "apply", "Apply")
	components, directive := componentArgCompletion(cmd, []string{"already-provided"}, "")
	assert.Nil(t, components)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestComponentArgCompletionResolvesConfiguredComponents(t *testing.T) {
	originalInit, originalDescribe, originalList := cfnInitCliConfig, cfnDescribeStacks, cfnListAllComponents
	t.Cleanup(func() {
		cfnInitCliConfig = originalInit
		cfnDescribeStacks = originalDescribe
		cfnListAllComponents = originalList
	})

	cmd := newOperationCommand("apply", "apply", "Apply")
	cfnInitCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	cfnDescribeStacks = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager) (map[string]any, error) {
		return map[string]any{"dev": map[string]any{}}, nil
	}
	cfnListAllComponents = func(context.Context, string, map[string]any) ([]string, error) {
		return []string{"stack-a", "stack-b"}, nil
	}

	components, directive := componentArgCompletion(cmd, nil, "")
	assert.Equal(t, []string{"stack-a", "stack-b"}, components)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)

	t.Run("configuration error", func(t *testing.T) {
		cfnInitCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, errors.New("config failed")
		}
		components, _ := componentArgCompletion(cmd, nil, "")
		assert.Nil(t, components)
	})

	t.Run("describe error", func(t *testing.T) {
		cfnInitCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		}
		cfnDescribeStacks = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager) (map[string]any, error) {
			return nil, errors.New("describe failed")
		}
		components, _ := componentArgCompletion(cmd, nil, "")
		assert.Nil(t, components)
	})

	t.Run("list error", func(t *testing.T) {
		cfnDescribeStacks = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager) (map[string]any, error) {
			return map[string]any{}, nil
		}
		cfnListAllComponents = func(context.Context, string, map[string]any) ([]string, error) {
			return nil, errors.New("list failed")
		}
		components, _ := componentArgCompletion(cmd, nil, "")
		assert.Nil(t, components)
	})
}

func TestValidateOperationArgs(t *testing.T) {
	tests := []struct {
		name    string
		command *cobra.Command
		args    []string
		wantErr string
	}{
		{
			name:    "single component",
			command: newOperationCommand("apply", "apply", "Apply"),
			args:    []string{"app"},
		},
		{
			name:    "all with no component",
			command: configuredOperationCommand(t, "apply", map[string]string{"all": "true"}),
		},
		{
			name:    "affected with no component",
			command: configuredOperationCommand(t, "apply", map[string]string{"affected": "true"}),
		},
		{
			name:    "all and affected are mutually exclusive",
			command: configuredOperationCommand(t, "apply", map[string]string{"all": "true", "affected": "true"}),
			wantErr: "--all and --affected are mutually exclusive",
		},
		{
			name:    "component cannot be combined with all",
			command: configuredOperationCommand(t, "apply", map[string]string{"all": "true"}),
			args:    []string{"app"},
			wantErr: "component argument cannot be used with --all, --affected, --tags, or --labels",
		},
		{
			name:    "missing component",
			command: newOperationCommand("apply", "apply", "Apply"),
			wantErr: "requires exactly one component argument unless --all, --affected, --tags, or --labels is set",
		},
		{
			name:    "too many components",
			command: newOperationCommand("apply", "apply", "Apply"),
			args:    []string{"app", "other"},
			wantErr: "requires exactly one component argument unless --all, --affected, --tags, or --labels is set",
		},
		{
			name:    "tags with no component",
			command: configuredOperationCommand(t, "apply", map[string]string{"tags": "production"}),
		},
		{
			name:    "labels with no component",
			command: configuredOperationCommand(t, "apply", map[string]string{"labels": "cost-center=platform"}),
		},
		{
			name:    "component cannot be combined with tags",
			command: configuredOperationCommand(t, "apply", map[string]string{"tags": "production"}),
			args:    []string{"app"},
			wantErr: "component argument cannot be used with --all, --affected, --tags, or --labels",
		},
		{
			name:    "component cannot be combined with labels",
			command: configuredOperationCommand(t, "apply", map[string]string{"labels": "cost-center=platform"}),
			args:    []string{"app"},
			wantErr: "component argument cannot be used with --all, --affected, --tags, or --labels",
		},
		{
			name:    "malformed labels flag errors",
			command: configuredOperationCommand(t, "apply", map[string]string{"labels": "not-valid"}),
			wantErr: "invalid label",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOperationArgs(tt.command, tt.args)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestGetOperationFlags(t *testing.T) {
	cmd := configuredOperationCommand(t, "delete", map[string]string{
		"all":                            "true",
		"affected":                       "false",
		"include-dependents":             "true",
		"clone-target-ref":               "true",
		"auto-approve":                   "true",
		"disable-termination-protection": "true",
		"repo-path":                      "/repo",
		"base":                           "main",
		"ref":                            "feature",
		"sha":                            "abc123",
		"ssh-key":                        "/tmp/key",
		"ssh-key-password":               "secret",
		"retain-resources":               "logical-id-1,logical-id-2",
	})

	flags := getOperationFlags(cmd)

	assert.Equal(t, true, flags["all"])
	assert.Equal(t, false, flags["affected"])
	assert.Equal(t, true, flags["include-dependents"])
	assert.Equal(t, true, flags["clone-target-ref"])
	assert.Equal(t, true, flags["auto-approve"])
	assert.Equal(t, true, flags["disable-termination-protection"])
	assert.Equal(t, "/repo", flags["repo-path"])
	assert.Equal(t, "main", flags["base"])
	assert.Equal(t, "feature", flags["ref"])
	assert.Equal(t, "abc123", flags["sha"])
	assert.Equal(t, "/tmp/key", flags["ssh-key"])
	assert.Equal(t, "secret", flags["ssh-key-password"])
	assert.Equal(t, []string{"logical-id-1", "logical-id-2"}, flags["retain-resources"])
	// target/format weren't registered on the delete command, so must be absent.
	assert.NotContains(t, flags, "target")
	assert.NotContains(t, flags, "format")
}

func TestGetOperationFlagsOmitsEmptyRetainResources(t *testing.T) {
	cmd := newOperationCommand("delete", "delete", "Delete")

	flags := getOperationFlags(cmd)

	assert.NotContains(t, flags, "retain-resources")
}

func TestGetOperationFlagsSurfacesOutputOptions(t *testing.T) {
	cmd := configuredOperationCommand(t, "output", map[string]string{
		"format":    "json",
		"flatten":   "true",
		"uppercase": "true",
	})

	flags := getOperationFlags(cmd)

	assert.Equal(t, "json", flags["format"])
	assert.Equal(t, true, flags["flatten"])
	assert.Equal(t, true, flags["uppercase"])
}

func TestBuildConfigAndStacksInfoPopulatesTagsAndLabels(t *testing.T) {
	cmd := configuredOperationCommand(t, "apply", map[string]string{
		"tags":   "production,tier-1",
		"labels": "cost-center=platform, compliance = sox",
	})

	info := buildConfigAndStacksInfo(cmd)

	assert.Equal(t, []string{"production", "tier-1"}, info.Tags)
	assert.Equal(t, map[string]string{"cost-center": "platform", "compliance": "sox"}, info.Labels)
	assert.True(t, info.ProcessTemplates)
	assert.True(t, info.ProcessFunctions)
}

func TestBuildConfigAndStacksInfoWithNoTagsOrLabels(t *testing.T) {
	cmd := newOperationCommand("apply", "apply", "Apply")

	info := buildConfigAndStacksInfo(cmd)

	assert.Empty(t, info.Tags)
	assert.Empty(t, info.Labels)
}

func TestApplySelectionFlagsReadsStackDryRunAllAffected(t *testing.T) {
	cmd := newOperationCommand("apply", "apply", "Apply")
	cmd.Flags().String("stack", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	require.NoError(t, cmd.Flags().Set("stack", "tenant-env-stage"))
	require.NoError(t, cmd.Flags().Set("dry-run", "true"))
	require.NoError(t, cmd.Flags().Set("all", "true"))

	info := buildConfigAndStacksInfo(cmd)

	assert.Equal(t, "tenant-env-stage", info.Stack)
	assert.True(t, info.DryRun)
	assert.True(t, info.All)
	assert.False(t, info.Affected)
}

func TestInitConfigAndStacksInfo(t *testing.T) {
	t.Setenv("ATMOS_IDENTITY", "dev-admin")
	cmd := configuredOperationCommand(t, "apply", map[string]string{"all": "true"})
	cmd.Flags().String("stack", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	require.NoError(t, cmd.Flags().Set("stack", "tenant-env-stage"))
	require.NoError(t, cmd.Flags().Set("dry-run", "true"))

	// The global --identity flag binds ATMOS_IDENTITY via Viper (see pkg/flags/global_builder.go),
	// so initConfigAndStacksInfo picks up the identity from the environment through that binding.
	cmd.Flags().StringP("identity", "i", "", "Specify identity")
	v := viper.GetViper()
	require.NoError(t, v.BindEnv("identity", "ATMOS_IDENTITY"))
	t.Cleanup(func() { v.Set("identity", nil) })

	info := initConfigAndStacksInfo(cmd, "apply", []string{"app"})

	assert.Equal(t, cfg.CloudFormationComponentType, info.ComponentType)
	assert.Equal(t, "apply", info.SubCommand)
	assert.Equal(t, []string{cfg.CloudFormationComponentType, "apply"}, info.CliArgs)
	assert.Equal(t, "app", info.ComponentFromArg)
	assert.Equal(t, "tenant-env-stage", info.Stack)
	assert.True(t, info.DryRun)
	assert.True(t, info.All)
	assert.Equal(t, "dev-admin", info.Identity)
}

func TestInitConfigAndStacksInfoNoArgs(t *testing.T) {
	cmd := newOperationCommand("render", "render", "Render")

	info := initConfigAndStacksInfo(cmd, "render", nil)

	assert.Equal(t, cfg.CloudFormationComponentType, info.ComponentType)
	assert.Equal(t, "render", info.SubCommand)
	assert.Empty(t, info.ComponentFromArg)
}

func TestApplySelectionFlagsReadsAffected(t *testing.T) {
	cmd := newOperationCommand("apply", "apply", "Apply")
	cmd.Flags().String("stack", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	require.NoError(t, cmd.Flags().Set("affected", "true"))

	info := buildConfigAndStacksInfo(cmd)

	assert.True(t, info.Affected)
	assert.False(t, info.All)
}

func TestNewOperationCommandRunEInvokesRunOperation(t *testing.T) {
	original, hadOriginal := component.GetProvider(cfg.CloudFormationComponentType)
	fake := &recordingProvider{}
	require.NoError(t, component.Register(fake))
	t.Cleanup(func() {
		if hadOriginal {
			require.NoError(t, component.Register(original))
		}
	})

	cmd := newOperationCommand("apply", "apply", "Apply")
	require.NotNil(t, cmd.RunE)

	require.NoError(t, cmd.RunE(cmd, []string{"app"}))
	require.Len(t, fake.executed, 1)
	assert.Equal(t, "app", fake.executed[0].Component)
	assert.Equal(t, "apply", fake.executed[0].SubCommand)
}

func TestRunOperationDelegatesToRegisteredProvider(t *testing.T) {
	original, hadOriginal := component.GetProvider(cfg.CloudFormationComponentType)
	fake := &recordingProvider{}
	require.NoError(t, component.Register(fake))
	t.Cleanup(func() {
		if hadOriginal {
			require.NoError(t, component.Register(original))
		}
	})

	cmd := configuredOperationCommand(t, "output", map[string]string{"format": "json"})

	require.NoError(t, runOperation(cmd, "output", []string{"app"}))
	require.Len(t, fake.executed, 1)
	ctx := fake.executed[0]
	assert.Equal(t, cfg.CloudFormationComponentType, ctx.ComponentType)
	assert.Equal(t, "app", ctx.Component)
	assert.Equal(t, "output", ctx.SubCommand)
	assert.Equal(t, []string{"app"}, ctx.Args)
	assert.Equal(t, "json", ctx.Flags["format"])
	assert.Equal(t, "app", ctx.ConfigAndStacksInfo.ComponentFromArg)
}

// The fmt operation command must register the fmt-only --check flag (via
// operationSpecificFlagOptions's "fmt" case), defaulting to false.
func TestOperationSpecificFlagOptions_Fmt_RegistersCheckFlag(t *testing.T) {
	fmtCmd := newOperationCommand("fmt", "fmt", "Format the local template in place")

	checkFlag := fmtCmd.Flag("check")
	require.NotNil(t, checkFlag, "expected fmt to register --check")
	assert.Equal(t, "false", checkFlag.DefValue)

	// --check is fmt-only: an unrelated operation must not pick it up.
	applyCmd := newOperationCommand("apply", subCommandApply, "Create or update the stack")
	assert.Nil(t, applyCmd.Flag("check"), "--check must be fmt-only")
}

// getOperationFlags must surface fmt's --check flag as a bool, both when set
// and when left at its default.
func TestGetOperationFlags_IncludesCheck(t *testing.T) {
	fmtCmd := newOperationCommand("fmt", "fmt", "Format the local template in place")
	require.NoError(t, fmtCmd.Flags().Set("check", "true"))

	flags := getOperationFlags(fmtCmd)
	assert.Equal(t, true, flags["check"])

	fmtCmdDefault := newOperationCommand("fmt", "fmt", "Format the local template in place")
	flags = getOperationFlags(fmtCmdDefault)
	assert.Equal(t, false, flags["check"])
}

// CloudFormationCmd must mount the fmt subcommand, registered via
// subCommandOperations' "fmt" -> OperationFmt entry (exercised end-to-end
// through cmd registration rather than the internal map directly, since the
// map itself lives in pkg/component/aws/cloudformation and is covered there).
func TestCloudFormationCmd_RegistersFmtSubcommand(t *testing.T) {
	var found *cobra.Command
	for _, sub := range CloudFormationCmd.Commands() {
		if sub.Name() == "fmt" {
			found = sub
		}
	}
	require.NotNil(t, found, "expected `atmos aws cloudformation fmt` to be registered")
	assert.NotNil(t, found.Flag("check"))
}

// CloudFormationCmd must mount the stackset verb group with its four
// subcommands (create/update/delete/instances).
func TestCloudFormationCmd_RegistersStackSetSubcommand(t *testing.T) {
	var found *cobra.Command
	for _, sub := range CloudFormationCmd.Commands() {
		if sub.Name() == "stackset" {
			found = sub
		}
	}
	require.NotNil(t, found, "expected `atmos aws cloudformation stackset` to be registered")

	names := make([]string, 0, len(found.Commands()))
	for _, sub := range found.Commands() {
		names = append(names, sub.Name())
	}
	assert.ElementsMatch(t, []string{"create", "update", "delete", "instances"}, names)
}

// CloudFormationCmd must mount tree/logs/watch as top-level subcommands.
func TestCloudFormationCmd_RegistersObservabilitySubcommands(t *testing.T) {
	names := make([]string, 0, len(CloudFormationCmd.Commands()))
	for _, sub := range CloudFormationCmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.Contains(t, names, "tree")
	assert.Contains(t, names, "logs")
	assert.Contains(t, names, "watch")
}

// newStackSetCmd's create/update subcommands must register --auto-approve
// (defaulting false, unlike deploy) and --target; delete must register only
// --auto-approve; instances must register neither.
func TestNewStackSetCmd_RegistersFlags(t *testing.T) {
	cmd := newStackSetCmd()

	var create, update, del, instances *cobra.Command
	for _, sub := range cmd.Commands() {
		switch sub.Name() {
		case "create":
			create = sub
		case "update":
			update = sub
		case "delete":
			del = sub
		case "instances":
			instances = sub
		}
	}
	require.NotNil(t, create)
	require.NotNil(t, update)
	require.NotNil(t, del)
	require.NotNil(t, instances)

	for _, sub := range []*cobra.Command{create, update} {
		autoApprove := sub.Flag(flagAutoApprove)
		require.NotNil(t, autoApprove, "%s must register --auto-approve", sub.Name())
		assert.Equal(t, "false", autoApprove.DefValue)
		assert.NotNil(t, sub.Flag("target"), "%s must register --target", sub.Name())
	}

	deleteAutoApprove := del.Flag(flagAutoApprove)
	require.NotNil(t, deleteAutoApprove, "delete must register --auto-approve")
	assert.Nil(t, del.Flag("target"), "delete must not register --target")

	assert.Nil(t, instances.Flag(flagAutoApprove), "instances is read-only and must not register --auto-approve")
	assert.Nil(t, instances.Flag("target"), "instances must not register --target")
}

// operationSpecificFlagOptions must delegate stackset/observability
// operations to phase3FlagOptions, registering "logs"'s --chart flag
// (defaulting false) and nothing extra for an unrecognized subCommand.
func TestOperationSpecificFlagOptions_DelegatesToPhase3(t *testing.T) {
	logsCmd := newOperationCommand("logs", "logs", "Show the combined event log")
	chartFlag := logsCmd.Flag("chart")
	require.NotNil(t, chartFlag, "expected logs to register --chart")
	assert.Equal(t, "false", chartFlag.DefValue)

	// --chart is logs-only: an unrelated operation must not pick it up.
	treeCmd := newOperationCommand("tree", "tree", "Render the nested-stack dependency tree")
	assert.Nil(t, treeCmd.Flag("chart"), "--chart must be logs-only")
}

// phase3FlagOptions must return nil (no extra flags) for a subCommand it
// doesn't recognize, keeping tree/watch flag-free beyond the shared set.
func TestPhase3FlagOptions_UnrecognizedSubCommand(t *testing.T) {
	assert.Nil(t, phase3FlagOptions("tree"))
	assert.Nil(t, phase3FlagOptions("watch"))
	assert.Nil(t, phase3FlagOptions("not-a-real-subcommand"))
}

// getOperationFlags must surface logs' --chart flag as a bool, both when set
// and when left at its default.
func TestGetOperationFlags_IncludesChart(t *testing.T) {
	logsCmd := newOperationCommand("logs", "logs", "Show the combined event log")
	require.NoError(t, logsCmd.Flags().Set("chart", "true"))

	flags := getOperationFlags(logsCmd)
	assert.Equal(t, true, flags["chart"])

	logsCmdDefault := newOperationCommand("logs", "logs", "Show the combined event log")
	flags = getOperationFlags(logsCmdDefault)
	assert.Equal(t, false, flags["chart"])
}
