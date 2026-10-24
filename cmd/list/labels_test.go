package list

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/tags"
)

func TestListOptions_ScalarLabelsPreserveSpaces(t *testing.T) {
	cmd := &cobra.Command{Use: "list"}
	v := viper.New()
	v.Set(labelsViperKey, "team = platform,owner=platform engineering")
	for name, labels := range map[string][]string{
		"components":   parseComponentsOptions(cmd, v).LabelsRaw,
		"dependencies": parseDependenciesOptions(cmd, v, nil).LabelsRaw,
		"instances":    parseInstancesOptions(cmd, v).LabelsRaw,
		"metadata":     parseMetadataOptions(cmd, v).LabelsRaw,
		"sources":      parseSourcesOptions(cmd, v, nil).LabelsRaw,
		"stacks":       parseStacksOptions(cmd, v).LabelsRaw,
	} {
		t.Run(name, func(t *testing.T) {
			parsed, err := tags.ParseLabelsFlag(labels)
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"team": "platform", "owner": "platform engineering"}, parsed)
		})
	}
}

// TestAffectedLabels_RepeatedFlagJoins proves that list affected, which keeps its labels as one
// comma-separated string, still sees every occurrence of the repeatable --labels flag through the
// namespaced Viper key instead of silently reading an empty string from the slice value.
func TestAffectedLabels_RepeatedFlagJoins(t *testing.T) {
	cmd := newCmdWithListParser("affected", affectedParser.RegisterFlags)
	setFlag(t, cmd, "labels", "team=platform")
	setFlag(t, cmd, "labels", "owner=platform engineering")
	v := viper.New()
	require.NoError(t, affectedParser.BindFlagsToViper(cmd, v))

	assert.Equal(t, "team=platform,owner=platform engineering", strings.Join(tags.ReadLabelsFlagKey(v, labelsViperKey), ","))
}

// TestListLabelsPreserveLiteralQuotes exercises real flag parsing before Viper and label decoding.
func TestListLabelsPreserveLiteralQuotes(t *testing.T) {
	for name, parser := range map[string]*flags.StandardParser{
		"instances": instancesParser, "components": componentsParser, "stacks": stacksParser,
	} {
		t.Run(name, func(t *testing.T) {
			cmd := newCmdWithListParser(name, parser.RegisterFlags)
			require.NoError(t, cmd.ParseFlags([]string{"--labels", `owner=O"Reilly,team=platform`, "--labels", "region=us-east-1"}))
			v := viper.New()
			require.NoError(t, parser.BindFlagsToViper(cmd, v))
			got, err := tags.ParseLabelsFlag(tags.ReadLabelsFlagKey(v, labelsViperKey))
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"owner": `O"Reilly`, "team": "platform", "region": "us-east-1"}, got)
		})
	}
}

func TestListLabelsSourcePrecedence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config any
		env    string
		args   []string
		want   map[string]string
	}{
		{name: "default"},
		{name: "scalar config", config: `owner=O"Reilly,team=platform engineering`, want: map[string]string{"owner": `O"Reilly`, "team": "platform engineering"}},
		{name: "list config", config: []string{`owner=O"Reilly`, "team=platform"}, want: map[string]string{"owner": `O"Reilly`, "team": "platform"}},
		{name: "environment overrides config", config: "owner=config", env: `owner=O"Reilly`, want: map[string]string{"owner": `O"Reilly`}},
		{name: "flags override environment", config: "owner=config", env: "owner=environment", args: []string{"--labels", `owner=O"Reilly,team=platform`, "--labels", "owner=cli"}, want: map[string]string{"owner": "cli", "team": "platform"}},
		{name: "explicit empty overrides environment", env: "owner=environment", args: []string{"--labels="}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ATMOS_COMPONENT_LABELS", tc.env)
			cmd := newCmdWithListParser("instances", instancesParser.RegisterFlags)
			require.NoError(t, cmd.ParseFlags(tc.args))
			v := viper.New()
			if tc.config != nil {
				require.NoError(t, v.MergeConfigMap(map[string]any{"list": map[string]any{"labels": tc.config}}))
			}
			require.NoError(t, instancesParser.BindFlagsToViper(cmd, v))
			got, err := tags.ParseLabelsFlag(tags.ReadLabelsFlagKey(v, labelsViperKey))
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
