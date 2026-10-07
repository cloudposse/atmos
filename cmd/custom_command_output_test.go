package cmd

import (
	"fmt"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCustomCommandShellViewport(t *testing.T) {
	for _, live := range []bool{false, true} {
		t.Run(fmt.Sprint(live), func(t *testing.T) {
			_ = NewTestKit(t)
			// Direct command.Run skips logger setup; isolate exact output from earlier tests.
			originalLevel := log.GetLevel()
			log.SetLevel(log.WarnLevel)
			t.Cleanup(func() { log.SetLevel(originalLevel) })
			// Other tests reset Viper, discarding the binding registered by init.
			require.NoError(t, viper.BindEnv("force-tty", "ATMOS_FORCE_TTY"))
			// Scope terminal overrides to the subtest; Viper overrides outlive TestKit.
			t.Setenv("ATMOS_FORCE_TTY", fmt.Sprint(live))
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

// TestCustomCommandShellViewport_InheritedDebug checks output assertions after a preceding
// command test has enabled debug logging, and verifies the original level is restored.
func TestCustomCommandShellViewport_InheritedDebug(t *testing.T) {
	originalLevel := log.GetLevel()
	t.Cleanup(func() { log.SetLevel(originalLevel) })
	log.SetLevel(log.DebugLevel)
	t.Run("viewport", TestCustomCommandShellViewport)
	assert.Equal(t, log.DebugLevel, log.GetLevel())
}
