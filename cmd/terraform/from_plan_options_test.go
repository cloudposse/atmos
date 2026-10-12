package terraform

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCobraFromPlanOptions(t *testing.T) {
	for _, test := range []struct {
		name    string
		args    []string
		usePlan bool
		path    string
	}{
		{name: "absent"},
		{name: "bare", args: []string{"--from-plan"}, usePlan: true},
		{name: "explicit empty", args: []string{"--from-plan="}, usePlan: true},
		{name: "path", args: []string{"--from-plan=reviewed.plan"}, usePlan: true, path: "reviewed.plan"},
		{name: "planfile", args: []string{"--planfile=selected.plan"}, usePlan: true, path: "selected.plan"},
		{name: "planfile precedence", args: []string{"--from-plan=other.plan", "--planfile=selected.plan"}, usePlan: true, path: "selected.plan"},
	} {
		t.Run(test.name, func(t *testing.T) {
			flags := pflag.NewFlagSet("saved-plan", pflag.ContinueOnError)
			flags.String("from-plan", "", "saved plan")
			flags.Lookup("from-plan").NoOptDefVal = "true"
			flags.String("planfile", "", "explicit plan")
			require.NoError(t, flags.Parse(test.args))
			v := viper.New()
			v.SetDefault("from-plan", "")
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

func TestFromPlanEnvironment(t *testing.T) {
	t.Setenv("ATMOS_TERRAFORM_APPLY_FROM_PLAN", "true")
	v := viper.New()
	require.NoError(t, v.BindEnv("from-plan", "ATMOS_TERRAFORM_APPLY_FROM_PLAN"))
	opts, err := ParseTerraformRunOptions(v)
	require.NoError(t, err)
	var info schema.ConfigAndStacksInfo
	applyOptionsToInfo(&info, opts)
	require.True(t, info.UseTerraformPlan)
	require.Empty(t, info.PlanFile)
}
