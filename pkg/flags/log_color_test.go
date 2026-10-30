package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggingColorFlagPrecedence(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  string
		args []string
		want bool
	}{
		{"default", "", nil, true},
		{"environment", "false", nil, false},
		{"CLI overrides environment", "false", []string{"--logs-color=true"}, true},
		{"bare flag", "false", []string{"--logs-color", "version"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_LOGS_COLOR", tt.env)
			cmd := &cobra.Command{Use: "test"}
			v := viper.New()
			parser := NewGlobalOptionsBuilder().Build()
			parser.RegisterPersistentFlags(cmd)
			require.NoError(t, parser.BindToViper(v))
			require.NoError(t, cmd.ParseFlags(tt.args))
			require.NoError(t, parser.BindFlagsToViper(cmd, v))
			assert.Equal(t, tt.want, ParseGlobalFlags(cmd, v).LogsColor)
		})
	}
}
