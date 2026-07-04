package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerfileInstallsPython3Runtime(t *testing.T) {
	content, err := os.ReadFile("../Dockerfile")
	require.NoError(t, err)

	assert.Contains(t, string(content), "--no-install-recommends curl git ca-certificates python3")
	assert.NotContains(t, string(content), "python3-pip")
	assert.NotContains(t, string(content), "python3-venv")
}

func TestAtmosCacheActionValidatesMetadataBeforeActionsCache(t *testing.T) {
	content, err := os.ReadFile("../actions/cache/action.yml")
	require.NoError(t, err)
	action := string(content)

	assert.Contains(t, action, "steps.meta.outputs.key")
	assert.Contains(t, action, "steps.meta.outputs.path")
	assert.Contains(t, action, "Atmos cache metadata did not include a cache key")
	assert.Contains(t, action, "Atmos cache metadata did not include any cache paths")
}
