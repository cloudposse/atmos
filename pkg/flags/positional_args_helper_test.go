package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestFirstPositionalArg(t *testing.T) {
	cmd := testPositionalArgCommand()

	tests := []struct {
		name string
		cmd  *cobra.Command
		args []string
		want string
	}{
		{
			name: "first arg is positional",
			cmd:  cmd,
			args: []string{"deploy", "--stack", "dev"},
			want: "deploy",
		},
		{
			name: "long flag with separated value",
			cmd:  cmd,
			args: []string{"--var", "secret=x", "vpc"},
			want: "vpc",
		},
		{
			name: "long flag assignment does not consume following positional",
			cmd:  cmd,
			args: []string{"--var=secret=x", "vpc"},
			want: "vpc",
		},
		{
			name: "bool flag does not consume following positional",
			cmd:  cmd,
			args: []string{"--dry-run", "vpc"},
			want: "vpc",
		},
		{
			name: "no opt flag does not consume following positional",
			cmd:  cmd,
			args: []string{"--optional", "vpc"},
			want: "vpc",
		},
		{
			name: "unknown flag consumes following value",
			cmd:  cmd,
			args: []string{"--token", "secret", "vpc"},
			want: "vpc",
		},
		{
			name: "attached shorthand value does not consume following positional",
			cmd:  cmd,
			args: []string{"-csecret", "vpc"},
			want: "vpc",
		},
		{
			name: "single dash is not treated as positional",
			cmd:  cmd,
			args: []string{"-", "vpc"},
			want: "vpc",
		},
		{
			name: "end of options stops scan",
			cmd:  cmd,
			args: []string{"--", "secret"},
			want: "",
		},
		{
			name: "nil command returns plain positional",
			cmd:  nil,
			args: []string{"deploy"},
			want: "deploy",
		},
		{
			name: "nil command treats unknown flag conservatively",
			cmd:  nil,
			args: []string{"--token", "secret", "deploy"},
			want: "deploy",
		},
		{
			name: "nil args",
			cmd:  cmd,
			args: nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FirstPositionalArg(tt.cmd, tt.args))
		})
	}
}

func TestFirstPositionalArgFlagHelpers(t *testing.T) {
	cmd := testPositionalArgCommand()

	assert.True(t, flagConsumesNextValue(cmd, "--var"))
	assert.False(t, flagConsumesNextValue(cmd, "--var=value"))
	assert.False(t, flagConsumesNextValue(cmd, "-cvalue"))
	assert.False(t, flagConsumesNextValue(cmd, "--dry-run"))
	assert.False(t, flagConsumesNextValue(cmd, "--optional"))
	assert.True(t, flagConsumesNextValue(cmd, "--unknown"))
	assert.False(t, flagConsumesNextValue(cmd, "-"))
	assert.False(t, flagConsumesNextValue(cmd, "--"))

	assert.False(t, hasAttachedShorthandValue(cmd, "-"))
	assert.False(t, hasAttachedShorthandValue(nil, "-cvalue"))
	assert.False(t, flagRequiresNextValue(cmd.Flags().Lookup("dry-run")))

	assert.Equal(t, cmd.Flags().Lookup("var"), lookupFlagForArg(cmd, "--var=value"))
	assert.Equal(t, cmd.Flags().ShorthandLookup("c"), lookupFlagForArg(cmd, "-c"))
	assert.Equal(t, cmd.Flags().Lookup("config"), lookupFlagForArg(cmd, "-config=value"))
	assert.Equal(t, cmd.Flags().ShorthandLookup("c"), lookupFlagForArg(cmd, "-cvalue"))
	assert.Nil(t, lookupFlagForArg(nil, "--var"))
	assert.Nil(t, lookupFlagForArg(cmd, "-"))

	assert.True(t, flagConsumesAttachedValue(cmd.Flags().Lookup("config")))
	assert.False(t, flagConsumesAttachedValue(cmd.Flags().Lookup("dry-run")))
	assert.False(t, flagConsumesAttachedValue(cmd.Flags().Lookup("optional")))
	assert.False(t, flagConsumesAttachedValue(nil))
}

func testPositionalArgCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "plan"}
	cmd.Flags().String("var", "", "")
	cmd.Flags().StringP("config", "c", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("optional", "", "")
	cmd.Flags().Lookup("optional").NoOptDefVal = "true"
	return cmd
}
