package list

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
