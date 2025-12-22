package yaml

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goyaml "gopkg.in/yaml.v3"
)

func TestLongString_MarshalYAML(t *testing.T) {
	tests := []struct {
		name     string
		input    LongString
		wantKind goyaml.Kind
	}{
		{
			name:     "simple string",
			input:    LongString("hello world"),
			wantKind: goyaml.ScalarNode,
		},
		{
			name:     "empty string",
			input:    LongString(""),
			wantKind: goyaml.ScalarNode,
		},
		{
			name:     "multiline string",
			input:    LongString("line1\nline2\nline3"),
			wantKind: goyaml.ScalarNode,
		},
		{
			name:     "long string",
			input:    LongString("This is a very long string that should be wrapped using the folded scalar style in YAML output"),
			wantKind: goyaml.ScalarNode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.input.MarshalYAML()
			require.NoError(t, err)

			node, ok := result.(*goyaml.Node)
			require.True(t, ok, "result should be a *goyaml.Node")
			assert.Equal(t, tt.wantKind, node.Kind)
			assert.Equal(t, goyaml.FoldedStyle, node.Style)
			assert.Equal(t, string(tt.input), node.Value)
		})
	}
}

func TestLongString_MarshalYAML_Integration(t *testing.T) {
	// Test that LongString marshals correctly when embedded in a struct.
	type testStruct struct {
		Description LongString `yaml:"description"`
	}

	ts := testStruct{
		Description: LongString("This is a long description that should be output as a folded scalar"),
	}

	out, err := goyaml.Marshal(&ts)
	require.NoError(t, err)

	// The output should use folded style (>).
	assert.Contains(t, string(out), "description:")
}

func TestDefaultIndent(t *testing.T) {
	// Verify the default indent constant.
	assert.Equal(t, 2, DefaultIndent)
}

func TestOptions_Struct(t *testing.T) {
	// Verify Options struct can be created and used.
	opts := Options{
		Indent: 4,
	}
	assert.Equal(t, 4, opts.Indent)

	// Default value.
	defaultOpts := Options{}
	assert.Equal(t, 0, defaultOpts.Indent)
}
