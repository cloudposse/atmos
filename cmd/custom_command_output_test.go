package cmd

import (
	"fmt"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCustomCommandShellViewport(t *testing.T) {
	for _, live := range []bool{false, true} {
		t.Run(fmt.Sprint(live), func(t *testing.T) {
			_ = NewTestKit(t)
			viper.Set("force-tty", live)
			config := schema.AtmosConfiguration{
				BasePath: t.TempDir(),
				Commands: []schema.Command{{Name: "viewport-check", Steps: schema.Tasks{{
					Type: "shell", Name: "compile", Output: "viewport",
					Viewport: &schema.ViewportConfig{Height: 4},
					Command:  "printf passing-stdout; printf passing-stderr >&2",
				}}}},
			}
			require.NoError(t, processCustomCommands(config, config.Commands, RootCmd))
			command, _, err := RootCmd.Find([]string{"viewport-check"})
			require.NoError(t, err)
			stdout, stderr := captureStdoutStderr(t, func() { command.Run(command, nil) })
			if live {
				assert.Empty(t, stdout, "successful viewport output must not leak to stdout")
				assert.NotContains(t, stderr, "passing-stderr")
				assert.Contains(t, stderr, "compile completed")
			} else {
				assert.Equal(t, "passing-stdout", stdout)
				assert.Equal(t, "passing-stderr", stderr)
			}
		})
	}
}
