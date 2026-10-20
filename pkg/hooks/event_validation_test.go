package hooks

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRunAllRejectsUnknownEventsBeforeFiltering(t *testing.T) {
	for _, pin := range []string{"", "2026-10-18", "2026-10-19", "2026-10", "2026", "invalid"} {
		t.Run("edition="+pin, func(t *testing.T) {
			config := &schema.AtmosConfiguration{Edition: pin}
			info := &schema.ConfigAndStacksInfo{}
			hookSet := &Hooks{items: map[string]Hook{
				"typo": {Events: []string{"after.terraform.aply"}, When: schema.MustCondition("false")},
			}}
			cmd := &cobra.Command{}
			cmd.Flags().String("skip-hooks", "*", "")
			err := hookSet.RunAll(BeforeTerraformPlan, config, info, cmd, nil)
			if pin == "2026-10-18" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			if pin != "invalid" {
				require.Contains(t, err.Error(), "typo")
				require.Contains(t, err.Error(), "after.terraform.aply")
			}
		})
	}
}
