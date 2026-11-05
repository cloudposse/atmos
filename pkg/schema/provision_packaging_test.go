package schema

import (
	"encoding/json"
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// TestProvisionPackagingRoundTrip verifies target selection survives typed configuration decoding.
func TestProvisionPackagingRoundTrip(t *testing.T) {
	input := map[string]any{"kind": "aws/cloudformation", "packaging": "artifacts"}
	decoders := map[string]func(*ProvisionTarget) error{
		"json": func(target *ProvisionTarget) error {
			return json.Unmarshal([]byte(`{"kind":"aws/cloudformation","packaging":"artifacts"}`), target)
		},
		"yaml": func(target *ProvisionTarget) error {
			return yaml.Unmarshal([]byte("kind: aws/cloudformation\npackaging: artifacts\n"), target)
		},
		"mapstructure": func(target *ProvisionTarget) error {
			return mapstructure.Decode(input, target)
		},
	}
	for name, decode := range decoders {
		t.Run(name, func(t *testing.T) {
			var target ProvisionTarget
			require.NoError(t, decode(&target))
			encoded, err := json.Marshal(target)
			require.NoError(t, err)
			var output map[string]any
			require.NoError(t, json.Unmarshal(encoded, &output))
			require.Equal(t, input["kind"], output["kind"])
			require.Equal(t, input["packaging"], output["packaging"])
		})
	}
}
