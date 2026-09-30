package list

import (
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
	v.Set("labels", "team = platform,owner=platform engineering")
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
