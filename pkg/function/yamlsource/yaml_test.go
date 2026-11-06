package yamlsource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// restoreAndReparse restores a transported value, emits it as YAML, and parses the output again,
// mirroring a YAML template pass over manifest data.
func restoreAndReparse(t *testing.T, value string) (string, *yaml.Node) {
	t.Helper()

	var node yaml.Node
	require.NoError(t, node.Encode(map[string]any{"value": value}))
	RestoreTags(&node)
	out, err := yaml.Marshal(&node)
	require.NoError(t, err)

	var document yaml.Node
	require.NoError(t, yaml.Unmarshal(out, &document))
	require.Len(t, document.Content, 1)
	mapping := document.Content[0]
	require.Len(t, mapping.Content, 2)
	return string(out), mapping.Content[1]
}

func TestRestoreTagsKeepsSequenceArgumentsAsSequences(t *testing.T) {
	encoded := (Source{Tag: "!ci.labels", Argument: `["preview","deploy"]`, Sequence: true}).Encode()

	out, restored := restoreAndReparse(t, encoded)

	assert.Equal(t, "value: !ci.labels [\"preview\", \"deploy\"]\n", out)
	assert.Equal(t, yaml.SequenceNode, restored.Kind)
	assert.Equal(t, "!ci.labels", restored.Tag)
	require.Len(t, restored.Content, 2)
	assert.Equal(t, "preview", restored.Content[0].Value)
	assert.Equal(t, "deploy", restored.Content[1].Value)
}

func TestRestoreTagsKeepsScalarArgumentsAsQuotedScalars(t *testing.T) {
	tests := []struct {
		name   string
		source Source
	}{
		{"scalar source", Source{Tag: "!env", Argument: "HOME"}},
		// A sequence flag on an argument that is not a flow sequence falls back to the scalar form.
		{"invalid sequence argument", Source{Tag: "!ci.labels", Argument: "preview", Sequence: true}},
		{"unparsable sequence argument", Source{Tag: "!ci.labels", Argument: "[preview", Sequence: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, restored := restoreAndReparse(t, tt.source.Encode())

			assert.Equal(t, yaml.ScalarNode, restored.Kind)
			assert.Equal(t, tt.source.Tag, restored.Tag)
			assert.Equal(t, tt.source.Argument, restored.Value)
		})
	}
}

func TestSequenceFlagRoundTripsThroughTransport(t *testing.T) {
	decoded, ok := Decode((Source{Tag: "!ci.labels", Argument: `["preview"]`, Sequence: true}).Encode())

	require.True(t, ok)
	assert.True(t, decoded.Sequence)
	assert.Equal(t, `!ci.labels ["preview"]`, decoded.Expression())
}
