package helm

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

func TestCommandProviderMetadata(t *testing.T) {
	provider := &CommandProvider{}

	require.Same(t, helmCmd, provider.GetCommand())
	assert.Equal(t, "helm", provider.GetName())
	assert.Equal(t, "Core Stack Commands", provider.GetGroup())
	assert.Nil(t, provider.GetAliases())
	assert.Nil(t, provider.GetFlagsBuilder())
	assert.Nil(t, provider.GetPositionalArgsBuilder())
	assert.Nil(t, provider.GetCompatibilityFlags())
	assert.True(t, provider.IsExperimental())

	var subcommands []string
	for _, cmd := range provider.GetCommand().Commands() {
		subcommands = append(subcommands, cmd.Name())
	}
	assert.ElementsMatch(t, []string{"template", "diff", "plan", "apply", "deploy", "delete", "plugin", "repo"}, subcommands)
}

func TestNewOperationCommandRegistersExpectedFlags(t *testing.T) {
	templateCmd := newOperationCommand("template", "Render")
	for _, name := range []string{"namespace", "all", "affected", "include-dependents", "repo-path", "base", "ref", "sha", "ssh-key", "ssh-key-password", "clone-target-ref", "output", "output-dir", "split", "tags", "labels"} {
		assert.NotNil(t, templateCmd.Flag(name), "expected template flag %q", name)
	}

	applyCmd := newOperationCommand("apply", "Apply")
	for _, name := range []string{"namespace", "target", "on-failure", "cleanup-on-failure", "wait", "wait-for-jobs", "timeout", "history-max", "no-hooks", "skip-crds"} {
		assert.NotNil(t, applyCmd.Flag(name), "expected apply flag %q", name)
	}
	assert.Equal(t, "watcher", applyCmd.Flag("wait").NoOptDefVal)
	assert.Nil(t, applyCmd.Flag("output"))
	assert.Nil(t, applyCmd.Flag("split"))
	assert.NotNil(t, applyCmd.ValidArgsFunction)
	require.NoError(t, applyCmd.Args(applyCmd, nil), "the missing component must reach the interactive prompt flow")
	require.Error(t, applyCmd.Args(applyCmd, []string{"app", "extra"}))

	// template does not get --target; apply/deploy do.
	assert.Nil(t, templateCmd.Flag("target"))
	assert.Nil(t, templateCmd.Flag("wait"))

	deleteCmd := newOperationCommand("delete", "Delete")
	for _, name := range []string{"namespace", "wait", "timeout", "no-hooks"} {
		assert.NotNil(t, deleteCmd.Flag(name), "expected delete flag %q", name)
	}
	assert.Nil(t, deleteCmd.Flag("on-failure"))

	// diff/plan get the baseline-selection flags; other operations do not.
	for _, opName := range []string{"diff", "plan"} {
		opCmd := newOperationCommand(opName, opName)
		for _, name := range []string{"against", "from-manifest", "context"} {
			assert.NotNil(t, opCmd.Flag(name), "expected %q flag on %q", name, opName)
		}
	}
	assert.Nil(t, applyCmd.Flag("against"))
	assert.Nil(t, templateCmd.Flag("from-manifest"))
}

func TestBareWaitDoesNotConsumeComponentArgument(t *testing.T) {
	for _, operation := range []string{"apply", "deploy", "delete"} {
		t.Run(operation, func(t *testing.T) {
			cmd := newOperationCommand(operation, operation)
			require.NoError(t, cmd.ParseFlags([]string{"--wait", "app"}))
			assert.Equal(t, "watcher", cmd.Flag("wait").Value.String())
			assert.Equal(t, []string{"app"}, cmd.Flags().Args())
		})
	}
}

func TestGetOperationFlagsIncludesOnlyExplicitLifecycleFlags(t *testing.T) {
	cmd := configuredOperationCommand(t, "apply", map[string]string{
		"namespace":          "incident-ns",
		"on-failure":         "rollback",
		"cleanup-on-failure": "true",
		"wait":               "legacy",
		"wait-for-jobs":      "true",
		"timeout":            "15m",
		"history-max":        "0",
		"no-hooks":           "true",
		"skip-crds":          "true",
	})

	actual := getOperationFlags(cmd)
	assert.Equal(t, "incident-ns", actual["namespace"])
	assert.Equal(t, "rollback", actual[cfg.HelmOnFailureSectionName])
	assert.Equal(t, true, actual[cfg.HelmCleanupOnFailureSectionName])
	assert.Equal(t, "legacy", actual[cfg.HelmWaitStrategySectionName])
	assert.Equal(t, true, actual[cfg.HelmWaitJobsSectionName])
	assert.Equal(t, "15m", actual[cfg.HelmTimeoutSectionName])
	assert.Equal(t, 0, actual[cfg.HelmHistoryMaxSectionName])
	assert.Equal(t, false, actual[cfg.HelmChartHooksSectionName])
	assert.Equal(t, "skip", actual[cfg.HelmCRDsSectionName])

	defaults := getOperationFlags(newOperationCommand("apply", "Apply"))
	assert.NotContains(t, defaults, "namespace")
	for _, key := range []string{
		cfg.HelmOnFailureSectionName,
		cfg.HelmCleanupOnFailureSectionName,
		cfg.HelmWaitStrategySectionName,
		cfg.HelmWaitJobsSectionName,
		cfg.HelmTimeoutSectionName,
		cfg.HelmHistoryMaxSectionName,
		cfg.HelmChartHooksSectionName,
		cfg.HelmCRDsSectionName,
	} {
		assert.NotContains(t, defaults, key)
	}
}

func TestOnFailureFlagUsesSingleActionAndCanClear(t *testing.T) {
	cmd := newOperationCommand("apply", "Apply")
	require.NoError(t, cmd.ParseFlags([]string{"--on-failure=rollback"}))
	assert.Equal(t, "rollback", getOperationFlags(cmd)[cfg.HelmOnFailureSectionName])

	cleared := newOperationCommand("apply", "Apply")
	require.NoError(t, cleared.ParseFlags([]string{"--on-failure="}))
	assert.Empty(t, getOperationFlags(cleared)[cfg.HelmOnFailureSectionName])
}

func TestSelectionFlagsAndComponentCompletion(t *testing.T) {
	for _, flag := range []string{"all", "affected", "tags", "labels"} {
		t.Run(flag, func(t *testing.T) {
			cmd := newOperationCommand("apply", "Apply")
			assert.False(t, hasSelectionFlags(cmd))
			if flag == "all" || flag == "affected" {
				require.NoError(t, cmd.Flags().Set(flag, "true"))
			} else {
				require.NoError(t, cmd.Flags().Set(flag, "value"))
			}
			assert.True(t, hasSelectionFlags(cmd))
		})
	}

	cmd := newOperationCommand("apply", "Apply")
	components, directive := componentArgCompletion(cmd, []string{"already-provided"}, "")
	assert.Nil(t, components)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestComponentArgCompletionResolvesConfiguredComponents(t *testing.T) {
	originalInit, originalDescribe, originalList := helmInitCliConfig, helmDescribeStacks, helmListAllComponents
	t.Cleanup(func() {
		helmInitCliConfig = originalInit
		helmDescribeStacks = originalDescribe
		helmListAllComponents = originalList
	})

	cmd := newOperationCommand("apply", "Apply")
	helmInitCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	helmDescribeStacks = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager) (map[string]any, error) {
		return map[string]any{"dev": map[string]any{}}, nil
	}
	helmListAllComponents = func(context.Context, string, map[string]any) ([]string, error) {
		return []string{"api", "worker"}, nil
	}

	components, directive := componentArgCompletion(cmd, nil, "")
	assert.Equal(t, []string{"api", "worker"}, components)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)

	t.Run("configuration error", func(t *testing.T) {
		helmInitCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, errors.New("config failed")
		}
		components, _ := componentArgCompletion(cmd, nil, "")
		assert.Nil(t, components)
	})

	t.Run("describe error", func(t *testing.T) {
		helmInitCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		}
		helmDescribeStacks = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager) (map[string]any, error) {
			return nil, errors.New("describe failed")
		}
		components, _ := componentArgCompletion(cmd, nil, "")
		assert.Nil(t, components)
	})

	t.Run("list error", func(t *testing.T) {
		helmDescribeStacks = func(*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager) (map[string]any, error) {
			return map[string]any{}, nil
		}
		helmListAllComponents = func(context.Context, string, map[string]any) ([]string, error) {
			return nil, errors.New("list failed")
		}
		components, _ := componentArgCompletion(cmd, nil, "")
		assert.Nil(t, components)
	})
}

func TestGetOperationFlagsIncludesDiffFlags(t *testing.T) {
	cmd := configuredOperationCommand(t, "diff", map[string]string{
		"against":       "target:prod",
		"from-manifest": "old.yaml",
		"context":       "7",
	})

	flags := getOperationFlags(cmd)
	assert.Equal(t, "target:prod", flags["against"])
	assert.Equal(t, "old.yaml", flags["from-manifest"])
	assert.Equal(t, 7, flags["context"])
}

func TestGetOperationFlagsIncludesAllOperationFlags(t *testing.T) {
	cmd := configuredOperationCommand(t, "template", map[string]string{
		"all":                "true",
		"affected":           "true",
		"ci":                 "true",
		"include-dependents": "true",
		"clone-target-ref":   "true",
		"repo-path":          "/repo",
		"base":               "origin/main",
		"ref":                "feature",
		"sha":                "abc123",
		"ssh-key":            "id_rsa",
		"ssh-key-password":   "secret",
		"output":             "rendered.yaml",
		"output-dir":         "rendered",
		"split":              "true",
	})

	flags := getOperationFlags(cmd)
	assert.Equal(t, true, flags["all"])
	assert.Equal(t, true, flags["affected"])
	assert.Equal(t, true, flags["ci"])
	assert.Equal(t, true, flags["include-dependents"])
	assert.Equal(t, true, flags["clone-target-ref"])
	assert.Equal(t, "/repo", flags["repo-path"])
	assert.Equal(t, "origin/main", flags["base"])
	assert.Equal(t, "feature", flags["ref"])
	assert.Equal(t, "abc123", flags["sha"])
	assert.Equal(t, "id_rsa", flags["ssh-key"])
	assert.Equal(t, "secret", flags["ssh-key-password"])
	assert.Equal(t, "rendered.yaml", flags["output"])
	assert.Equal(t, "rendered", flags["output_dir"])
	assert.Equal(t, true, flags["split"])
}

func TestGetOperationFlagsIgnoresInvalidContext(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("context", "not-an-int", "")
	flags := getOperationFlags(cmd)
	assert.NotContains(t, flags, "context")
}

func TestBuildAndInitConfigAndStacksInfo(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cmd := newOperationCommand("apply", "Apply")
	cmd.Flags().String("stack", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	require.NoError(t, cmd.Flags().Set("stack", "dev"))
	require.NoError(t, cmd.Flags().Set("dry-run", "true"))
	require.NoError(t, cmd.Flags().Set("all", "true"))
	require.NoError(t, cmd.Flags().Set("affected", "true"))

	info := buildConfigAndStacksInfo(cmd)
	assert.Equal(t, "dev", info.Stack)
	assert.True(t, info.DryRun)
	assert.True(t, info.All)
	assert.True(t, info.Affected)
	assert.True(t, info.ProcessTemplates)
	assert.True(t, info.ProcessFunctions)

	info = initConfigAndStacksInfo(cmd, "apply", []string{"app"})
	assert.Equal(t, cfg.HelmComponentType, info.ComponentType)
	assert.Equal(t, "apply", info.SubCommand)
	assert.Equal(t, []string{cfg.HelmComponentType, "apply"}, info.CliArgs)
	assert.Equal(t, "app", info.ComponentFromArg)
}

func TestRunOperationBuildsExecutionContext(t *testing.T) {
	provider := &capturingHelmProvider{}
	original, hadOriginal := component.GetProvider(cfg.HelmComponentType)
	require.NoError(t, component.Register(provider))
	t.Cleanup(func() {
		if hadOriginal {
			require.NoError(t, component.Register(original))
		}
	})

	cmd := configuredOperationCommand(t, "diff", map[string]string{
		"against": "target",
		"context": "5",
	})
	type contextKey struct{}
	cmd.SetContext(context.WithValue(context.Background(), contextKey{}, "command"))
	cmd.Flags().String("stack", "", "")
	require.NoError(t, cmd.Flags().Set("stack", "dev"))

	require.NoError(t, runOperation(cmd, "diff", []string{"app"}))
	require.NotNil(t, provider.ctx)
	assert.Equal(t, cfg.HelmComponentType, provider.ctx.ComponentType)
	assert.Equal(t, "app", provider.ctx.Component)
	assert.Equal(t, "dev", provider.ctx.Stack)
	assert.Equal(t, "diff", provider.ctx.SubCommand)
	assert.Equal(t, []string{"app"}, provider.ctx.Args)
	assert.Equal(t, "target", provider.ctx.Flags["against"])
	assert.Equal(t, 5, provider.ctx.Flags["context"])
	assert.Equal(t, "command", provider.ctx.GoContext().Value(contextKey{}))
}

func TestRunOperationBuildsApplyDryRunExecutionContext(t *testing.T) {
	provider := &capturingHelmProvider{}
	originalProviders := component.ListProviders()
	require.NoError(t, component.Register(provider))
	t.Cleanup(func() {
		component.Reset()
		for _, providers := range originalProviders {
			for _, original := range providers {
				require.NoError(t, component.Register(original))
			}
		}
	})

	cmd := newOperationCommand("apply", "Apply")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("stack", "", "")
	require.NoError(t, cmd.Flags().Set("dry-run", "true"))
	require.NoError(t, cmd.Flags().Set("stack", "dev"))

	require.NoError(t, runOperation(cmd, "apply", []string{"app"}))
	require.NotNil(t, provider.ctx)
	assert.Equal(t, "apply", provider.ctx.SubCommand)
	assert.True(t, provider.ctx.ConfigAndStacksInfo.DryRun)
}

type capturingHelmProvider struct {
	ctx *component.ExecutionContext
}

func (p *capturingHelmProvider) GetType() string { return cfg.HelmComponentType }

func (p *capturingHelmProvider) GetGroup() string                                { return "Kubernetes" }
func (p *capturingHelmProvider) GetBasePath(_ *schema.AtmosConfiguration) string { return "" }
func (p *capturingHelmProvider) ListComponents(context.Context, string, map[string]any) ([]string, error) {
	return nil, nil
}
func (p *capturingHelmProvider) ValidateComponent(map[string]any) error { return nil }
func (p *capturingHelmProvider) Execute(ctx *component.ExecutionContext) error {
	p.ctx = ctx
	return nil
}
func (p *capturingHelmProvider) GenerateArtifacts(*component.ExecutionContext) error { return nil }
func (p *capturingHelmProvider) GetAvailableCommands() []string                      { return nil }

func configuredOperationCommand(t *testing.T, name string, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := newOperationCommand(name, "test")
	for k, v := range flags {
		require.NoError(t, cmd.Flags().Set(k, v))
	}
	return cmd
}

func TestValidateOperationArgs(t *testing.T) {
	tests := []struct {
		name    string
		command *cobra.Command
		args    []string
		wantErr string
	}{
		{name: "single component", command: newOperationCommand("apply", "Apply"), args: []string{"app"}},
		{name: "all with no component", command: configuredOperationCommand(t, "apply", map[string]string{"all": "true"})},
		{name: "affected with no component", command: configuredOperationCommand(t, "apply", map[string]string{"affected": "true"})},
		{
			name:    "all and affected mutually exclusive",
			command: configuredOperationCommand(t, "apply", map[string]string{"all": "true", "affected": "true"}),
			wantErr: "--all and --affected are mutually exclusive",
		},
		{
			name:    "component cannot combine with all",
			command: configuredOperationCommand(t, "apply", map[string]string{"all": "true"}),
			args:    []string{"app"},
			wantErr: "component argument cannot be used with --all, --affected, --tags, or --labels",
		},
		{
			name:    "template bulk cannot use output",
			command: configuredOperationCommand(t, "template", map[string]string{"all": "true", "output": "rendered.yaml"}),
			wantErr: "--output and --output-dir are only supported when rendering one component",
		},
		{
			name:    "missing component",
			command: newOperationCommand("apply", "Apply"),
			wantErr: "requires exactly one component argument unless --all or --affected is set",
		},
		{
			name:    "too many components",
			command: newOperationCommand("apply", "Apply"),
			args:    []string{"app", "other"},
			wantErr: "requires exactly one component argument unless --all or --affected is set",
		},
		{name: "tags with no component", command: configuredOperationCommand(t, "apply", map[string]string{"tags": "production"})},
		{name: "labels with no component", command: configuredOperationCommand(t, "apply", map[string]string{"labels": "cost-center=platform"})},
		{
			name:    "component cannot combine with tags",
			command: configuredOperationCommand(t, "apply", map[string]string{"tags": "production"}),
			args:    []string{"app"},
			wantErr: "component argument cannot be used with --all, --affected, --tags, or --labels",
		},
		{
			name:    "component cannot combine with labels",
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

func TestBuildConfigAndStacksInfoPopulatesTagsAndLabels(t *testing.T) {
	cmd := configuredOperationCommand(t, "apply", map[string]string{
		"tags":   "production,tier-1",
		"labels": "cost-center=platform, compliance = sox",
	})

	info := buildConfigAndStacksInfo(cmd)

	assert.Equal(t, []string{"production", "tier-1"}, info.Tags)
	assert.Equal(t, map[string]string{"cost-center": "platform", "compliance": "sox"}, info.Labels)
}

func TestBuildConfigAndStacksInfoWithNoTagsOrLabels(t *testing.T) {
	cmd := newOperationCommand("apply", "Apply")

	info := buildConfigAndStacksInfo(cmd)

	assert.Empty(t, info.Tags)
	assert.Empty(t, info.Labels)
}
