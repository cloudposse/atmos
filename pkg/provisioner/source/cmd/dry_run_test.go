package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
)

func mountSourceCommand(t *testing.T, verb string, inherited bool) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "cloudformation", SilenceErrors: true, SilenceUsage: true}
	if inherited {
		flags.NewStandardParser(flags.WithDryRunFlag()).RegisterPersistentFlags(root)
	}
	group := &cobra.Command{Use: "source"}
	cfg := &Config{ComponentType: "aws/cloudformation", TypeLabel: "CloudFormation"}
	if verb == "pull" {
		group.AddCommand(PullCommand(cfg))
	} else {
		group.AddCommand(DeleteCommand(cfg))
	}
	root.AddCommand(group)
	return root
}

// TestSourceDryRun_ValidationBeforeSideEffects verifies --dry-run applies the same validation as a
// real run (component exists, declares source, destination resolvable, directory deletable) and
// skips only the side effects: authentication, provisioning and deletion.
func TestSourceDryRun_ValidationBeforeSideEffects(t *testing.T) {
	handmade := map[string]any{
		"x-handmade": map[string]any{"atmos_component": "x-handmade", "component": "vpc"},
	}
	cases := []struct {
		name     string
		verbs    []string
		args     []string
		scenario dryRunScenario
		want     error
	}{
		{name: "valid", verbs: []string{"pull", "delete"}, args: []string{"vpc", "--stack", "dev"}, scenario: dryRunScenario{section: vpcSection(nil), existing: true, provisioned: true}},
		{name: "empty component", verbs: []string{"pull", "delete"}, args: []string{"", "--stack", "dev"}, want: errUtils.ErrInvalidPositionalArgs},
		{name: "missing stack", verbs: []string{"pull", "delete"}, args: []string{"vpc"}, want: errUtils.ErrRequiredFlagNotProvided},
		{name: "component without source", verbs: []string{"pull", "delete"}, args: []string{"vpc", "--stack", "dev"}, scenario: dryRunScenario{section: map[string]any{"atmos_component": "vpc"}, existing: true, provisioned: true}, want: errUtils.ErrSourceMissing},
		{name: "nonexistent component", verbs: []string{"pull", "delete"}, args: []string{"vpc", "--stack", "dev"}, want: errUtils.ErrDescribeComponent},
		{name: "absolute metadata.component", verbs: []string{"pull", "delete"}, args: []string{"vpc", "--stack", "dev"}, scenario: dryRunScenario{section: vpcSection(map[string]any{"component": "/abs/path"})}, want: errUtils.ErrSourceComponentNameInvalid},
		{name: "directory without provenance", verbs: []string{"delete"}, args: []string{"vpc", "--stack", "dev"}, scenario: dryRunScenario{section: vpcSection(nil), existing: true}, want: errUtils.ErrSourceDeleteRefused},
		{name: "directory owned by sourceless component", verbs: []string{"delete"}, args: []string{"vpc", "--stack", "dev"}, scenario: dryRunScenario{section: vpcSection(nil), stackComponents: handmade, existing: true, provisioned: true}, want: errUtils.ErrSourceDeleteRefused},
	}
	for _, tt := range cases {
		for _, verb := range tt.verbs {
			t.Run(verb+"/"+tt.name, func(t *testing.T) {
				env := newDryRunEnv(t, tt.scenario)
				cmd := mountSourceCommand(t, verb, true)
				cmd.SetArgs(append([]string{"source", verb, "--dry-run"}, tt.args...))
				err := cmd.Execute()
				if tt.want != nil {
					require.ErrorIs(t, err, tt.want)
				} else {
					require.NoError(t, err)
				}
				assert.Zero(t, env.provisions, "dry-run must not provision")
				assert.Zero(t, env.authentications, "dry-run must not authenticate")
				if tt.scenario.existing {
					assert.DirExists(t, env.targetDir, "dry-run must not delete")
				}
			})
		}
	}
}

func TestSourceDryRun_DoesNotLeakFromViper(t *testing.T) {
	for _, verb := range []string{"pull", "delete"} {
		for _, inherited := range []bool{false, true} {
			t.Run(verb+"/"+map[bool]string{false: "absent flag", true: "explicit false"}[inherited], func(t *testing.T) {
				env := newDryRunEnv(t, dryRunScenario{section: vpcSection(nil), existing: true, provisioned: true})
				viper.Set("dry-run", true)
				cmd := mountSourceCommand(t, verb, inherited)
				args := []string{"source", verb, "vpc", "--stack", "dev", "--force"}
				if inherited {
					args = append(args, "--dry-run=false")
				}
				cmd.SetArgs(args)
				require.NoError(t, cmd.Execute())
				if verb == "pull" {
					assert.Equal(t, 1, env.provisions, "the real run provisions")
				} else {
					assert.NoDirExists(t, env.targetDir, "the real run deletes")
				}
			})
		}
	}
}
