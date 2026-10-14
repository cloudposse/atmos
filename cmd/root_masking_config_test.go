package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestReconcileMaskingRegistersResolvedConfig(t *testing.T) {
	NewTestKit(t)
	t.Cleanup(func() {
		iolib.Reset()
		viper.Reset()
	})
	for _, disabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "enabled", true: "local flag disables"}[disabled], func(t *testing.T) {
			iolib.Reset()
			viper.Reset()
			require.NoError(t, iolib.Initialize())
			cmd := &cobra.Command{Use: "workflow"}
			cmd.PersistentFlags().Bool("mask", true, "")
			if disabled {
				require.NoError(t, cmd.PersistentFlags().Set("mask", "false"))
			}
			var config schema.AtmosConfiguration
			config.Settings.Terminal.Mask = schema.MaskSettings{
				Enabled: true, Replacement: "REDACTED", Literals: []string{"field-secret"}, Patterns: []string{"token-[0-9]+"},
			}
			reconcileMaskingForCommand(cmd, &config)
			got := iolib.GetContext().Masker().Mask("field-secret token-123")
			if disabled {
				require.Equal(t, "field-secret token-123", got)
			} else {
				require.Equal(t, "REDACTED REDACTED", got)
			}
		})
	}
}
