package composition

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompositionCommandProvider(t *testing.T) {
	provider := &CompositionCommandProvider{}

	cmd := provider.GetCommand()
	require.NotNil(t, cmd)
	assert.Equal(t, "composition", cmd.Use)
	assert.Equal(t, "composition", provider.GetName())
	assert.Equal(t, "Core Stack Commands", provider.GetGroup())
	assert.Nil(t, provider.GetAliases())
	assert.Nil(t, provider.GetFlagsBuilder())
	assert.Nil(t, provider.GetPositionalArgsBuilder())
	assert.Nil(t, provider.GetCompatibilityFlags())
	assert.False(t, provider.IsExperimental())
}

func TestCompositionCommandStructure(t *testing.T) {
	subcommands := compositionCmd.Commands()
	names := make([]string, len(subcommands))
	for i, c := range subcommands {
		names[i] = c.Name()
	}
	assert.Contains(t, names, "list")
	assert.Contains(t, names, "validate")
	assert.Contains(t, names, "logs")
	for _, verb := range lifecycleVerbs {
		assert.Contains(t, names, verb)
	}
}

func TestValidateAcceptsOptionalCompositionArg(t *testing.T) {
	// validate [composition] accepts zero or one positional argument.
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "no args", args: []string{}, wantErr: false},
		{name: "exactly one arg", args: []string{"storefront"}, wantErr: false},
		{name: "too many args", args: []string{"a", "b"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCmd.Args(validateCmd, tt.args)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestLifecycleCommandsAcceptOptionalCompositionArg(t *testing.T) {
	for _, name := range append(lifecycleVerbs, "logs") {
		t.Run(name, func(t *testing.T) {
			cmd, _, err := compositionCmd.Find([]string{name})
			require.NoError(t, err)
			require.NotNil(t, cmd)
			require.NoError(t, cmd.Args(cmd, []string{}))
			require.NoError(t, cmd.Args(cmd, []string{"storefront"}))
			require.Error(t, cmd.Args(cmd, []string{"one", "two"}))
		})
	}
}

func TestCompositionVerbFlags(t *testing.T) {
	cmd := logsCmd
	require.NoError(t, cmd.Flags().Set("follow", "true"))
	require.NoError(t, cmd.Flags().Set("tail", "20"))
	t.Cleanup(func() {
		_ = cmd.Flags().Set("follow", "false")
		_ = cmd.Flags().Set("tail", "all")
	})

	flags := compositionVerbFlags(cmd)
	assert.Equal(t, true, flags["follow"])
	assert.Equal(t, "20", flags["tail"])
}

func TestBuildConfigAndStacksInfo_ResolvesStack(t *testing.T) {
	// Stack is resolved via viper so the full precedence chain is honored.
	resetCompositionViper(t)
	v := viper.GetViper()

	v.Set("stack", "dev")
	info := buildConfigAndStacksInfo(compositionCmd)
	assert.Equal(t, "dev", info.Stack)

	resetCompositionViper(t)
	info = buildConfigAndStacksInfo(compositionCmd)
	assert.Empty(t, info.Stack)
}

func TestBuildConfigAndStacksInfo_ResolvesStackAfterBindingFlags(t *testing.T) {
	resetCompositionViper(t)
	v := viper.GetViper()

	cmd := validateCmd
	require.NoError(t, compositionCmd.PersistentFlags().Set("stack", "local"))
	require.NoError(t, compositionParser.BindFlagsToViper(cmd, v))
	t.Cleanup(func() { _ = compositionCmd.PersistentFlags().Set("stack", "") })

	info := buildConfigAndStacksInfo(cmd)
	assert.Equal(t, "local", info.Stack)
}

func resetCompositionViper(t *testing.T) {
	t.Helper()
	viper.Reset()
	require.NoError(t, compositionParser.BindToViper(viper.GetViper()))
	t.Cleanup(func() {
		viper.Reset()
		_ = compositionParser.BindToViper(viper.GetViper())
	})
}
