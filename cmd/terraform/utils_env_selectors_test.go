package terraform

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newSelectorTestCmd returns a command that registers the --tags and --labels selector flags.
func newSelectorTestCmd(name string) *cobra.Command {
	cmd := &cobra.Command{Use: name}
	cmd.Flags().StringSlice("tags", nil, "")
	cmd.Flags().String("labels", "", "")
	return cmd
}

// setViperSelectors sets the bare tags/labels Viper keys (what ATMOS_TAGS / ATMOS_LABELS populate)
// and restores them when the test ends.
func setViperSelectors(t *testing.T, selectorTags []string, selectorLabels string) {
	t.Helper()

	v := viper.GetViper()
	t.Cleanup(func() {
		v.Set("tags", []string{})
		v.Set("labels", "")
	})
	v.Set("tags", selectorTags)
	v.Set("labels", selectorLabels)
}

// TestHasComponentArgument verifies detection of a component argument for plain and compound terraform subcommands.
func TestHasComponentArgument(t *testing.T) {
	tests := []struct {
		name       string
		subCommand string
		args       []string
		want       bool
	}{
		{name: "plain subcommand without component", subCommand: "plan", args: nil, want: false},
		{name: "plain subcommand with component", subCommand: "plan", args: []string{"vpc"}, want: true},
		{name: "compound subcommand without component", subCommand: "providers", args: []string{"lock"}, want: false},
		{name: "compound subcommand with component", subCommand: "providers", args: []string{"lock", "vpc"}, want: true},
		{name: "state subcommand without component", subCommand: "state", args: []string{"list"}, want: false},
		{name: "workspace subcommand with component", subCommand: "workspace", args: []string{"select", "vpc"}, want: true},
		{name: "bare compound command", subCommand: "workspace", args: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasComponentArgument(tt.subCommand, tt.args))
		})
	}
}

// TestIsMultiComponentInvocation_EnvOnlySelectors covers the env-only rule: ATMOS_TAGS/ATMOS_LABELS
// alone make an invocation multi-component only when no component argument was given.
func TestIsMultiComponentInvocation_EnvOnlySelectors(t *testing.T) {
	tests := []struct {
		name         string
		cmdName      string
		args         []string
		envTags      []string
		envLabels    string
		cliFlag      string // Flag set explicitly on the command line ("tags" or "labels").
		cliFlagValue string
		want         bool
	}{
		{name: "env tags, no component", cmdName: "plan", envTags: []string{"prod"}, want: true},
		{name: "env tags, component given", cmdName: "plan", args: []string{"vpc"}, envTags: []string{"prod"}, want: false},
		{name: "env labels, no component", cmdName: "plan", envLabels: "ci=auto", want: true},
		{name: "env labels, component given", cmdName: "plan", args: []string{"vpc"}, envLabels: "ci=auto", want: false},
		{name: "env tags and labels, component given", cmdName: "apply", args: []string{"vpc"}, envTags: []string{"prod"}, envLabels: "ci=auto", want: false},
		{name: "cli tags, component given", cmdName: "plan", args: []string{"vpc"}, cliFlag: "tags", cliFlagValue: "prod", want: true},
		{name: "cli labels, component given", cmdName: "plan", args: []string{"vpc"}, cliFlag: "labels", cliFlagValue: "ci=auto", want: true},
		{name: "env tags, compound command without component", cmdName: "providers", args: []string{"lock"}, envTags: []string{"prod"}, want: true},
		{name: "env tags, compound command with component", cmdName: "providers", args: []string{"lock", "vpc"}, envTags: []string{"prod"}, want: false},
		{name: "nothing set", cmdName: "plan", args: []string{"vpc"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setViperSelectors(t, tt.envTags, tt.envLabels)
			cmd := newSelectorTestCmd(tt.cmdName)
			if tt.cliFlag != "" {
				require.NoError(t, cmd.Flags().Set(tt.cliFlag, tt.cliFlagValue))
			}

			assert.Equal(t, tt.want, isMultiComponentInvocation(cmd, tt.args))
		})
	}
}

// TestDropEnvOnlySelectorsForComponent verifies that selectors sourced only from the environment are dropped when a component argument is given and kept otherwise.
func TestDropEnvOnlySelectorsForComponent(t *testing.T) {
	envLabels := map[string]string{"ci": "auto"}

	t.Run("env-only selectors are dropped when a component was given", func(t *testing.T) {
		cmd := newSelectorTestCmd("plan")
		info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"}
		opts := &TerraformRunOptions{Tags: []string{"prod"}, Labels: envLabels}

		dropEnvOnlySelectorsForComponent(cmd, info, opts)

		assert.Nil(t, opts.Tags)
		assert.Nil(t, opts.Labels)
	})

	t.Run("env-only selectors are kept when no component was given", func(t *testing.T) {
		cmd := newSelectorTestCmd("plan")
		info := &schema.ConfigAndStacksInfo{}
		opts := &TerraformRunOptions{Tags: []string{"prod"}, Labels: envLabels}

		dropEnvOnlySelectorsForComponent(cmd, info, opts)

		assert.Equal(t, []string{"prod"}, opts.Tags)
		assert.Equal(t, envLabels, opts.Labels)
	})

	t.Run("explicit --tags is kept and still conflicts with the component argument", func(t *testing.T) {
		cmd := newSelectorTestCmd("plan")
		require.NoError(t, cmd.Flags().Set("tags", "prod"))
		info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"}
		opts := &TerraformRunOptions{Tags: []string{"prod"}, Labels: envLabels}

		dropEnvOnlySelectorsForComponent(cmd, info, opts)
		assert.Equal(t, []string{"prod"}, opts.Tags)
		assert.Nil(t, opts.Labels, "the env-only labels are still dropped")

		applyOptionsToInfo(info, opts)
		err := checkTerraformFlags(info)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags)
	})

	t.Run("explicit --labels is kept and still conflicts with the component argument", func(t *testing.T) {
		cmd := newSelectorTestCmd("plan")
		require.NoError(t, cmd.Flags().Set("labels", "ci=auto"))
		info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"}
		opts := &TerraformRunOptions{Tags: []string{"prod"}, Labels: envLabels}

		dropEnvOnlySelectorsForComponent(cmd, info, opts)
		assert.Nil(t, opts.Tags, "the env-only tags are still dropped")
		assert.Equal(t, envLabels, opts.Labels)

		applyOptionsToInfo(info, opts)
		err := checkTerraformFlags(info)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags)
	})

	t.Run("dropped env selectors no longer conflict with the component argument", func(t *testing.T) {
		cmd := newSelectorTestCmd("plan")
		info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"}
		opts := &TerraformRunOptions{Tags: []string{"prod"}, Labels: envLabels}

		dropEnvOnlySelectorsForComponent(cmd, info, opts)
		applyOptionsToInfo(info, opts)

		assert.NoError(t, checkTerraformFlags(info))
	})
}
