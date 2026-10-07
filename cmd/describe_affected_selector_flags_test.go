package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
}

// TestDescribeAffectedCmd_RegistersSelectorFlags confirms the real command exposes both selectors.
func TestDescribeAffectedCmd_RegistersSelectorFlags(t *testing.T) {
	_ = NewTestKit(t)

	assert.NotNil(t, describeAffectedCmd.PersistentFlags().Lookup("tags"))
	assert.NotNil(t, describeAffectedCmd.PersistentFlags().Lookup("labels"))
}

func TestResolveDescribeAffectedSelectorFlags(t *testing.T) {
	tests := []struct {
		name       string
		cliArgs    []string
		env        map[string]string
		wantTags   []string
		wantLabels string
		tagsSet    bool
		labelsSet  bool
	}{
		{name: "nothing set leaves flags untouched", wantTags: []string{}},
		{
			name:       "env vars populate unset flags",
			env:        map[string]string{"ATMOS_TAGS": "prod,tier-1", "ATMOS_LABELS": "ci=auto"},
			wantTags:   []string{"prod", "tier-1"},
			wantLabels: "ci=auto",
			tagsSet:    true,
			labelsSet:  true,
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
			assert.Equal(t, tt.tagsSet, cmd.Flags().Changed("tags"))
			assert.Equal(t, tt.labelsSet, cmd.Flags().Changed("labels"))
		})
	}
}
