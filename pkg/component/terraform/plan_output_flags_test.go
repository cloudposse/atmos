package terraform

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Terraform parses its flags with Go's flag package, which accepts one or two leading dashes, so
// `--out=my.tfplan` and `--out my.tfplan` select the plan file exactly as `-out` does. Atmos must not
// add its own -out next to any of them: two output flags are rejected as an ambiguous plan destination.
func TestBuildPlanArgsHonorsEveryUserPlanOutputForm(t *testing.T) {
	for _, userArgs := range [][]string{
		{"-out", "my.tfplan"},
		{"-out=my.tfplan"},
		{"--out", "my.tfplan"},
		{"--out=my.tfplan"},
	} {
		t.Run(userArgs[0], func(t *testing.T) {
			atmosConfig := schema.AtmosConfiguration{}
			info := schema.ConfigAndStacksInfo{SubCommand: "plan", AdditionalArgsAndFlags: userArgs}
			componentPath := filepath.Join(t.TempDir(), "component")

			args, _, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "atmos.tfplan", &componentPath)
			require.NoError(t, err)
			require.NotContains(t, args, "atmos.tfplan", "Atmos must not add its own plan file when the user chose one")
			require.Equal(t, userArgs, args[len(args)-len(userArgs):], "the user's output flag reaches Terraform unchanged")

			// The command Terraform receives carries exactly one output flag: the user's.
			require.NoError(t, validatePlanOutputFlags("plan", args))
			require.Equal(t, filepath.Join(componentPath, "my.tfplan"), plannedOutputPath(componentPath, args))
		})
	}
}

func TestBuildPlanArgsAddsPlanFileWithoutUserOutput(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	// A flag that merely starts with "out" is not an output flag.
	info := schema.ConfigAndStacksInfo{SubCommand: "plan", AdditionalArgsAndFlags: []string{"-outline"}}
	componentPath := filepath.Join(t.TempDir(), "component")

	args, _, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "atmos.tfplan", &componentPath)
	require.NoError(t, err)
	require.Contains(t, args, outFlag)
	require.Contains(t, args, "atmos.tfplan")
}

func TestValidatePlanOutputFlagsRejectsMixedDashForms(t *testing.T) {
	require.Error(t, validatePlanOutputFlags("plan", []string{"plan", "-out", "a.tfplan", "--out=b.tfplan"}))
	require.Error(t, validatePlanOutputFlags("plan", []string{"plan", "--out", "a.tfplan", "-out=b.tfplan"}))
	require.NoError(t, validatePlanOutputFlags("plan", []string{"plan", "--out", "a.tfplan"}))
	require.NoError(t, validatePlanOutputFlags("apply", []string{"apply", "-out=a", "-out=b"}))
}
