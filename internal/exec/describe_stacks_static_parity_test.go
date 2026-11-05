package exec

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStaticStackNameMatchesRuntimeDelimiters(t *testing.T) {
	for _, source := range []string{"{{ .vars.stage }}", "[[ .vars.stage ]]"} {
		t.Run(source, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			config.Stacks.NameTemplate = source
			config.Templates.Settings.Delimiters = []string{"[[", "]]"}
			info := schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"vars": map[string]any{"stage": "dev"}}}
			normal, _, err := ResolveStackName(config, "file", "", info, nil)
			require.NoError(t, err)
			processor := &describeStacksProcessor{atmosConfig: config, staticOnly: true}
			static, _, err := processor.resolveStackName("file", "", info, nil)
			require.NoError(t, err)
			require.Equal(t, normal, static)
		})
	}
}
