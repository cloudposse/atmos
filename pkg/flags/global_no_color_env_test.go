package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGlobalNoColorEnvBindings verifies which environment variables feed the no-color key.
// CLICOLOR only means "color is supported"; it must never be read as a no-color opt-out.
func TestGlobalNoColorEnvBindings(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"nothing set", nil, false},
		{"CLICOLOR=1 is not no-color", map[string]string{"CLICOLOR": "1"}, false},
		{"CLICOLOR=0 is handled by terminal detection, not no-color", map[string]string{"CLICOLOR": "0"}, false},
		{"ATMOS_NO_COLOR=true", map[string]string{"ATMOS_NO_COLOR": "true"}, true},
		{"NO_COLOR=1", map[string]string{"NO_COLOR": "1"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"ATMOS_NO_COLOR", "NO_COLOR", "CLICOLOR"} {
				t.Setenv(key, "")
			}
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			cmd := &cobra.Command{Use: "test"}
			v := viper.New()
			parser := NewGlobalOptionsBuilder().Build()
			parser.RegisterPersistentFlags(cmd)
			require.NoError(t, parser.BindToViper(v))
			require.NoError(t, parser.BindFlagsToViper(cmd, v))

			assert.Equal(t, tt.want, v.GetBool("no-color"))
		})
	}
}
