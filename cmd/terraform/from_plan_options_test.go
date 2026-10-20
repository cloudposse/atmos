package terraform

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCobraFromPlanOptions verifies optional values and explicit planfile precedence.
func TestCobraFromPlanOptions(t *testing.T) {
	for _, test := range []struct {
		name        string
		args        []string
		environment string
		usePlan     bool
		path        string
	}{
		{name: "absent"},
		{name: "bare", args: []string{"--from-plan"}, usePlan: true},
		{name: "explicit true", args: []string{"--from-plan=true"}, usePlan: true},
		{name: "explicit false", args: []string{"--from-plan=false"}},
		{name: "flag disables environment selection", environment: "true", args: []string{"--from-plan=false"}},
		{name: "flag disables environment path", environment: "reviewed.plan", args: []string{"--from-plan=false"}},
		{name: "flag enables disabled environment", environment: "false", args: []string{"--from-plan"}, usePlan: true},
		{name: "explicit empty", args: []string{"--from-plan="}, usePlan: true},
		{name: "path", args: []string{"--from-plan=reviewed.plan"}, usePlan: true, path: "reviewed.plan"},
		{name: "planfile", args: []string{"--planfile=selected.plan"}, usePlan: true, path: "selected.plan"},
		{name: "planfile precedence", args: []string{"--from-plan=other.plan", "--planfile=selected.plan"}, usePlan: true, path: "selected.plan"},
		{name: "planfile with false", args: []string{"--from-plan=false", "--planfile=selected.plan"}, usePlan: true, path: "selected.plan"},
		{name: "false after planfile", args: []string{"--planfile=selected.plan", "--from-plan=false"}, usePlan: true, path: "selected.plan"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ATMOS_TERRAFORM_APPLY_FROM_PLAN", test.environment)
			flags := pflag.NewFlagSet("saved-plan", pflag.ContinueOnError)
			flags.String("from-plan", "", "saved plan")
			flags.Lookup("from-plan").NoOptDefVal = "true"
			flags.String("planfile", "", "explicit plan")
			require.NoError(t, flags.Parse(test.args))
			v := viper.New()
			v.SetDefault("from-plan", "")
			require.NoError(t, v.BindEnv("from-plan", "ATMOS_TERRAFORM_APPLY_FROM_PLAN"))
			require.NoError(t, v.BindPFlags(flags))
			opts, err := ParseTerraformRunOptions(v)
			require.NoError(t, err)
			var info schema.ConfigAndStacksInfo
			applyOptionsToInfo(&info, opts)
			command := &cobra.Command{}
			command.Flags().AddFlagSet(flags)
			applyFromPlanSelection(&info, command)
			require.Equal(t, test.usePlan, info.UseTerraformPlan)
			require.Equal(t, test.path, info.PlanFile)
		})
	}
}

// TestFromPlanEnvironment verifies boolean sentinels and paths for apply and deploy.
func TestFromPlanEnvironment(t *testing.T) {
	for _, env := range []string{"ATMOS_TERRAFORM_APPLY_FROM_PLAN", "ATMOS_TERRAFORM_DEPLOY_FROM_PLAN"} {
		for _, test := range []struct {
			value   string
			usePlan bool
			path    string
		}{
			{value: "true", usePlan: true},
			{value: "false"},
			{value: ""},
			{value: "reviewed.plan", usePlan: true, path: "reviewed.plan"},
		} {
			t.Run(env+"/"+test.value, func(t *testing.T) {
				t.Setenv(env, test.value)
				v := viper.New()
				require.NoError(t, v.BindEnv("from-plan", env))
				opts, err := ParseTerraformRunOptions(v)
				require.NoError(t, err)
				var info schema.ConfigAndStacksInfo
				applyOptionsToInfo(&info, opts)
				require.Equal(t, test.usePlan, info.UseTerraformPlan)
				require.Equal(t, test.path, info.PlanFile)
			})
		}
	}
}
