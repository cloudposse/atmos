package helm

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.ElementsMatch(t, []string{"template", "diff", "plan", "apply", "deploy", "delete", "plugin"}, subcommands)
}

func TestNewOperationCommandRegistersExpectedFlags(t *testing.T) {
	templateCmd := newOperationCommand("template", "Render")
	for _, name := range []string{"all", "affected", "include-dependents", "repo-path", "base", "ref", "sha", "ssh-key", "ssh-key-password", "clone-target-ref", "output", "output-dir", "split"} {
		assert.NotNil(t, templateCmd.Flag(name), "expected template flag %q", name)
	}

	applyCmd := newOperationCommand("apply", "Apply")
	assert.NotNil(t, applyCmd.Flag("target"))
	assert.Nil(t, applyCmd.Flag("output"))
	assert.Nil(t, applyCmd.Flag("split"))

	// template does not get --target; apply/deploy do.
	assert.Nil(t, templateCmd.Flag("target"))
}

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
			wantErr: "component argument cannot be used with --all or --affected",
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
