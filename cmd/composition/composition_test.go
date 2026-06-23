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
	assert.Contains(t, names, "validate")
}

func TestValidateRequiresExactlyOneArg(t *testing.T) {
	// validate <composition> takes exactly one positional argument.
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "no args", args: []string{}, wantErr: true},
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

func TestBuildConfigAndStacksInfo_ResolvesStack(t *testing.T) {
	// Stack is resolved via viper so the full precedence chain is honored.
	v := viper.GetViper()
	orig := v.GetString("stack")
	t.Cleanup(func() { v.Set("stack", orig) })

	v.Set("stack", "dev")
	info := buildConfigAndStacksInfo(compositionCmd)
	assert.Equal(t, "dev", info.Stack)

	v.Set("stack", "")
	info = buildConfigAndStacksInfo(compositionCmd)
	assert.Empty(t, info.Stack)
}
