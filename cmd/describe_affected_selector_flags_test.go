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

// newSelectorFlagsTestCmd builds a bare command with only the --tags and --labels flags registered
// as persistent flags (matching production) plus an isolated Viper, so tests don't leak state into
// the global Viper or the real describeAffectedCmd. The cliArgs parameter simulates the command line.
func newSelectorFlagsTestCmd(t *testing.T, cliArgs ...string) (*cobra.Command, *viper.Viper, *flags.StandardParser) {
	t.Helper()

	parser := newDescribeAffectedSelectorFlagsParser()
	cmd := &cobra.Command{Use: "affected"}
	parser.RegisterPersistentFlags(cmd)
	require.NoError(t, cmd.ParseFlags(cliArgs))

	v := viper.New()
	require.NoError(t, parser.BindToViper(v))
	return cmd, v, parser
}

// TestNewDescribeAffectedSelectorFlagsParser verifies that the selector parser registers the tags, labels, and flatten flags with the expected types and usage.
func TestNewDescribeAffectedSelectorFlagsParser(t *testing.T) {
	cmd := &cobra.Command{Use: "affected"}
	newDescribeAffectedSelectorFlagsParser().RegisterPersistentFlags(cmd)

	tags := cmd.PersistentFlags().Lookup(describeAffectedTagsFlagName)
	require.NotNil(t, tags)
	assert.Equal(t, "stringSlice", tags.Value.Type())
	assert.NotEmpty(t, tags.Usage)

	labels := cmd.PersistentFlags().Lookup(describeAffectedLabelsFlagName)
	require.NotNil(t, labels)
	assert.Equal(t, "string", labels.Value.Type())
	assert.NotEmpty(t, labels.Usage)

	flatten := cmd.PersistentFlags().Lookup(describeAffectedFlattenFlagName)
	require.NotNil(t, flatten)
	assert.Equal(t, "bool", flatten.Value.Type())
	assert.Equal(t, "false", flatten.DefValue)
	assert.NotEmpty(t, flatten.Usage)
}

// TestDescribeAffectedCmd_RegistersSelectorFlags confirms the real command exposes both selectors.
func TestDescribeAffectedCmd_RegistersSelectorFlags(t *testing.T) {
	_ = NewTestKit(t)

	assert.NotNil(t, describeAffectedCmd.PersistentFlags().Lookup("tags"))
	assert.NotNil(t, describeAffectedCmd.PersistentFlags().Lookup("labels"))
	assert.NotNil(t, describeAffectedCmd.PersistentFlags().Lookup("flatten"))
}

// TestResolveDescribeAffectedSelectorFlags verifies that tags, labels, and flatten resolve from CLI flags and environment variables and record the env var that supplied each value.
func TestResolveDescribeAffectedSelectorFlags(t *testing.T) {
	tests := []struct {
		name        string
		cliArgs     []string
		env         map[string]string
		wantTags    []string
		wantLabels  string
		wantFlatten bool
		tagsSet     bool
		labelsSet   bool
		flattenSet  bool
		// The env var recorded on each flag, or empty when the value did not come from the environment.
		wantTagsEnv    string
		wantLabelsEnv  string
		wantFlattenEnv string
	}{
		{name: "nothing set leaves flags untouched", wantTags: []string{}},
		{
			name:          "env vars populate unset flags",
			env:           map[string]string{"ATMOS_TAGS": "prod,tier-1", "ATMOS_LABELS": "ci=auto"},
			wantTags:      []string{"prod", "tier-1"},
			wantLabels:    "ci=auto",
			tagsSet:       true,
			labelsSet:     true,
			wantTagsEnv:   "ATMOS_TAGS",
			wantLabelsEnv: "ATMOS_LABELS",
		},
		{
			name:       "CLI flags win over env vars",
			cliArgs:    []string{"--tags=dev", "--labels=ci=manual"},
			env:        map[string]string{"ATMOS_TAGS": "prod", "ATMOS_LABELS": "ci=auto"},
			wantTags:   []string{"dev"},
			wantLabels: "ci=manual",
			tagsSet:    true,
			labelsSet:  true,
		},
		{
			name:       "CLI flags work without env vars",
			cliArgs:    []string{"--labels=ci=auto"},
			wantTags:   []string{},
			wantLabels: "ci=auto",
			labelsSet:  true,
		},
		{
			name:       "an explicitly empty CLI flag overrides the env var",
			cliArgs:    []string{"--tags=", "--labels="},
			env:        map[string]string{"ATMOS_TAGS": "prod", "ATMOS_LABELS": "ci=auto"},
			wantTags:   []string{},
			wantLabels: "",
			tagsSet:    true,
			labelsSet:  true,
		},
		{
			name:           "flatten from env",
			env:            map[string]string{"ATMOS_DESCRIBE_AFFECTED_FLATTEN": "true"},
			wantTags:       []string{},
			wantFlatten:    true,
			flattenSet:     true,
			wantFlattenEnv: "ATMOS_DESCRIBE_AFFECTED_FLATTEN",
		},
		{
			name:        "flatten from CLI",
			cliArgs:     []string{"--flatten"},
			wantTags:    []string{},
			wantFlatten: true,
			flattenSet:  true,
		},
		{
			name:        "flatten CLI wins over env",
			cliArgs:     []string{"--flatten=false"},
			env:         map[string]string{"ATMOS_DESCRIBE_AFFECTED_FLATTEN": "true"},
			wantTags:    []string{},
			wantFlatten: false,
			flattenSet:  true,
		},
		{
			name:     "flatten env false leaves the flag untouched",
			env:      map[string]string{"ATMOS_DESCRIBE_AFFECTED_FLATTEN": "false"},
			wantTags: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			cmd, v, p := newSelectorFlagsTestCmd(t, tt.cliArgs...)

			require.NoError(t, resolveDescribeAffectedSelectorFlags(cmd, v, p))

			gotTags, err := cmd.Flags().GetStringSlice("tags")
			require.NoError(t, err)
			assert.Equal(t, tt.wantTags, gotTags)
			gotLabels, err := cmd.Flags().GetString("labels")
			require.NoError(t, err)
			assert.Equal(t, tt.wantLabels, gotLabels)
			gotFlatten, err := cmd.Flags().GetBool("flatten")
			require.NoError(t, err)
			assert.Equal(t, tt.wantFlatten, gotFlatten)
			assert.Equal(t, tt.tagsSet, cmd.Flags().Changed("tags"))
			assert.Equal(t, tt.labelsSet, cmd.Flags().Changed("labels"))
			assert.Equal(t, tt.flattenSet, cmd.Flags().Changed("flatten"))

			// The annotation is set only for env-sourced values, never for explicit CLI flags.
			for flagName, want := range map[string]string{
				"tags":    tt.wantTagsEnv,
				"labels":  tt.wantLabelsEnv,
				"flatten": tt.wantFlattenEnv,
			} {
				envVar, ok := flags.FlagValueFromEnv(cmd.Flags().Lookup(flagName))
				assert.Equal(t, want != "", ok, "flag %s", flagName)
				assert.Equal(t, want, envVar, "flag %s", flagName)
			}
		})
	}
}

// TestResolveDescribeAffectedSelectorFlags_InvalidFlattenEnv rejects a non-boolean ATMOS_DESCRIBE_AFFECTED_FLATTEN.
func TestResolveDescribeAffectedSelectorFlags_InvalidFlattenEnv(t *testing.T) {
	t.Setenv("ATMOS_DESCRIBE_AFFECTED_FLATTEN", "yes-please")
	cmd, v, p := newSelectorFlagsTestCmd(t)

	err := resolveDescribeAffectedSelectorFlags(cmd, v, p)
	require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	assert.False(t, cmd.Flags().Changed("flatten"))
}

// TestResolveDescribeAffectedSelectorFlags_ClearsStaleEnvMark guards the long-lived command object
// reused across runs (for example by tests): a mark from an earlier env-sourced run must not survive
// a later run that supplies the value on the command line.
func TestResolveDescribeAffectedSelectorFlags_ClearsStaleEnvMark(t *testing.T) {
	cmd, v, p := newSelectorFlagsTestCmd(t, "--tags=dev")
	flags.MarkFlagValueFromEnv(cmd.Flags().Lookup("tags"), "ATMOS_TAGS")

	require.NoError(t, resolveDescribeAffectedSelectorFlags(cmd, v, p))

	_, ok := flags.FlagValueFromEnv(cmd.Flags().Lookup("tags"))
	assert.False(t, ok)
}
