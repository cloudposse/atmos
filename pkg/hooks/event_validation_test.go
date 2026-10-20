package hooks

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRunAllEventEditionConfigurationFallback(t *testing.T) {
	legacy := &schema.AtmosConfiguration{Edition: "2026-10-18"}
	current := &schema.AtmosConfiguration{Edition: "2026-10-19"}
	unpinned := &schema.AtmosConfiguration{}
	tests := []struct {
		name     string
		stored   *schema.AtmosConfiguration
		run      *schema.AtmosConfiguration
		wantFail bool
	}{
		{name: "no configuration", wantFail: true},
		{name: "stored legacy", stored: legacy},
		{name: "stored current", stored: current, wantFail: true},
		{name: "stored unpinned", stored: unpinned, wantFail: true},
		{name: "run legacy overrides stored current", stored: current, run: legacy},
		{name: "run current overrides stored legacy", stored: legacy, run: current, wantFail: true},
		{name: "run unpinned overrides stored legacy", stored: legacy, run: unpinned, wantFail: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hookSet := &Hooks{config: tt.stored, items: map[string]Hook{
				"typo": {Events: []string{"after.terraform.aply"}, When: schema.MustCondition("false")},
			}}
			err := hookSet.RunAll(BeforeTerraformPlan, tt.run, &schema.ConfigAndStacksInfo{}, nil, nil)
			if tt.wantFail {
				require.ErrorIs(t, err, errUtils.ErrUnsupportedHookEvent)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

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
