package policy

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/flags"
)

var checkParser = flags.NewStandardParser(
	flags.WithStringFlag("format", "f", "text", "Output format: text, json, yaml"),
	flags.WithEnvVars("format", "ATMOS_POLICY_CHECK_FORMAT", "ATMOS_POLICY_FORMAT"),
	flags.WithValidValues("format", "text", "json", "yaml"),
)

func newCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "check",
		Short:   "Validate the policy bundle without executing it",
		Long:    "Validate the policy bundle and display a human-readable summary.\nUse --format json or --format yaml for machine-readable output on stdout.",
		Example: "atmos policy check\natmos policy check --format json\natmos policy check --format yaml",
		Args:    cobra.NoArgs,
		RunE:    runCheck,
	}
	checkParser.RegisterFlags(cmd)
	return cmd
}
