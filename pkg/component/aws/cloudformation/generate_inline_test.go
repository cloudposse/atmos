package cloudformation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestInlineTemplateWithGeneratedAuxiliaryFiles(t *testing.T) {
	config, info := generationFixture(t, t.TempDir(), "inline")
	delete(info.ComponentSection, "path")
	info.ComponentSection["template"] = map[string]any{"Resources": map[string]any{}}
	info.ComponentSection["generate"].(map[string]any)["config.yaml"] = map[string]any{"value": "{{ .vars.value }}", "enabled": true}
	require.NoError(t, prepareGeneration(config, info))
	path, err := prepareComponentFiles(t.Context(), config, info)
	require.NoError(t, err)
	yamlBytes, err := os.ReadFile(filepath.Join(path, "config.yaml"))
	require.NoError(t, err)
	var data map[string]any
	require.NoError(t, yaml.Unmarshal(yamlBytes, &data))
	assert.Equal(t, map[string]any{"value": "inline", "enabled": true}, data)
	jsonBytes, err := os.ReadFile(filepath.Join(path, "policy.json"))
	require.NoError(t, err)
	require.True(t, json.Valid(jsonBytes))
	spec, err := resolveSpecAndTemplate(t.Context(), config, info, OperationApply)
	require.NoError(t, err)
	assert.Contains(t, spec.TemplateBody, "Resources")
	assert.Equal(t, string(jsonBytes), spec.StackPolicyBody)
}
