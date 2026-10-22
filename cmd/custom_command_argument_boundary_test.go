package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/customcommand"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCustomCommandDefaultsBeforeNativeArguments(t *testing.T) {
	for _, defaultValue := range []string{"", "site.yml"} {
		t.Run("default="+defaultValue, func(t *testing.T) {
			config := &schema.Command{
				Name:      "run",
				Arguments: []schema.CommandArgument{{Name: "playbook", Default: defaultValue}},
				Steps:     schema.Tasks{{Type: "shell", Command: "echo test"}},
			}
			parent := &cobra.Command{Use: "root"}
			command, err := createCustomCommand(&schema.AtmosConfiguration{}, config, parent)
			require.NoError(t, err)
			parent.AddCommand(command)
			require.NoError(t, command.ParseFlags([]string{"--", "--limit", "web"}))
			command.PreRun(command, command.Flags().Args())
			args, found, err := customcommand.DecodeArguments(command.Annotations[annotationResolvedArgs])
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, []string{defaultValue}, args)
		})
	}
}
