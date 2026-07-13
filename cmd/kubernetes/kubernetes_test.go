package kubernetes

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

type recordingProvider struct {
	executed []*component.ExecutionContext
}

func (p *recordingProvider) GetType() string { return cfg.KubernetesComponentType }

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

func TestCommandProviderMetadata(t *testing.T) {
	provider := &CommandProvider{}

	require.Same(t, kubernetesCmd, provider.GetCommand())
	assert.Equal(t, "kubernetes", provider.GetName())
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
	assert.ElementsMatch(t, []string{"render", "diff", "plan", "apply", "deploy", "delete", "validate"}, subcommands)
}

func TestNewOperationCommandRegistersExpectedFlags(t *testing.T) {
	renderCmd := newOperationCommand("render", "Render")
	for _, name := range []string{"all", "affected", "include-dependents", "repo-path", "base", "ref", "sha", "ssh-key", "ssh-key-password", "clone-target-ref", "output", "output-dir", "split"} {
		assert.NotNil(t, renderCmd.Flag(name), "expected render flag %q", name)
	}

	applyCmd := newOperationCommand("apply", "Apply")
	assert.NotNil(t, applyCmd.Flag("all"))
	assert.Nil(t, applyCmd.Flag("output"))
	assert.Nil(t, applyCmd.Flag("output-dir"))
	assert.Nil(t, applyCmd.Flag("split"))

	validateCmd := newOperationCommand("validate", "Validate")
	assert.NotNil(t, validateCmd.Flag("server"), "expected validate to register --server")
	assert.Nil(t, validateCmd.Flag("output"))
	assert.Nil(t, applyCmd.Flag("server"), "--server is validate-only")
}

func TestGetOperationFlagsSurfacesServer(t *testing.T) {
	cmd := configuredOperationCommand(t, "validate", map[string]string{"server": "true"})

	flags := getOperationFlags(cmd)

	assert.Equal(t, true, flags["server"])
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
			command: newOperationCommand("apply", "Apply"),
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
			wantErr: "component argument cannot be used with --all or --affected",
		},
		{
			name:    "render bulk cannot use output",
			command: configuredOperationCommand(t, "render", map[string]string{"all": "true", "output": "rendered.yaml"}),
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
	cmd := configuredOperationCommand(t, "render", map[string]string{
		"all":                "true",
		"affected":           "false",
		"ci":                 "true",
		"include-dependents": "true",
		"clone-target-ref":   "true",
		"repo-path":          "/repo",
		"base":               "main",
		"ref":                "feature",
		"sha":                "abc123",
		"ssh-key":            "/tmp/key",
		"ssh-key-password":   "secret",
		"output":             "rendered.yaml",
		"output-dir":         "manifests",
		"split":              "true",
	})

	flags := getOperationFlags(cmd)

	assert.Equal(t, true, flags["all"])
	assert.Equal(t, false, flags["affected"])
	assert.Equal(t, true, flags["ci"])
	assert.Equal(t, true, flags["include-dependents"])
	assert.Equal(t, true, flags["clone-target-ref"])
	assert.Equal(t, "/repo", flags["repo-path"])
	assert.Equal(t, "main", flags["base"])
	assert.Equal(t, "feature", flags["ref"])
	assert.Equal(t, "abc123", flags["sha"])
	assert.Equal(t, "/tmp/key", flags["ssh-key"])
	assert.Equal(t, "secret", flags["ssh-key-password"])
	assert.Equal(t, "rendered.yaml", flags["output"])
	assert.Equal(t, "manifests", flags["output_dir"])
	assert.Equal(t, true, flags["split"])
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

	assert.Equal(t, cfg.KubernetesComponentType, info.ComponentType)
	assert.Equal(t, "apply", info.SubCommand)
	assert.Equal(t, []string{cfg.KubernetesComponentType, "apply"}, info.CliArgs)
	assert.Equal(t, "app", info.ComponentFromArg)
	assert.Equal(t, "tenant-env-stage", info.Stack)
	assert.True(t, info.DryRun)
	assert.True(t, info.All)
	assert.Equal(t, "dev-admin", info.Identity)
	assert.True(t, info.ProcessTemplates)
	assert.True(t, info.ProcessFunctions)
}

func TestRunOperationDelegatesToRegisteredProvider(t *testing.T) {
	original, hadOriginal := component.GetProvider(cfg.KubernetesComponentType)
	fake := &recordingProvider{}
	require.NoError(t, component.Register(fake))
	t.Cleanup(func() {
		if hadOriginal {
			require.NoError(t, component.Register(original))
		}
	})

	cmd := configuredOperationCommand(t, "render", map[string]string{"output": "rendered.yaml"})

	require.NoError(t, runOperation(cmd, "render", []string{"app"}))
	require.Len(t, fake.executed, 1)
	ctx := fake.executed[0]
	assert.Equal(t, cfg.KubernetesComponentType, ctx.ComponentType)
	assert.Equal(t, "app", ctx.Component)
	assert.Equal(t, "render", ctx.SubCommand)
	assert.Equal(t, []string{"app"}, ctx.Args)
	assert.Equal(t, "rendered.yaml", ctx.Flags["output"])
	assert.Equal(t, "app", ctx.ConfigAndStacksInfo.ComponentFromArg)
}

func configuredOperationCommand(t *testing.T, name string, values map[string]string) *cobra.Command {
	t.Helper()

	cmd := newOperationCommand(name, name)
	for flagName, value := range values {
		require.NoError(t, cmd.Flags().Set(flagName, value))
	}
	return cmd
}
