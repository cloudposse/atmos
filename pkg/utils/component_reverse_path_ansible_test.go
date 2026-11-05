package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractAnsibleComponentPaths(t *testing.T) {
	root := t.TempDir()
	component := filepath.Join(root, "components", "ansible", "demo")
	require.NoError(t, os.MkdirAll(component, 0o755))
	config := &schema.AtmosConfiguration{BasePath: root}
	config.Components.Ansible.BasePath = filepath.Join("components", "ansible")
	t.Chdir(root)
	for _, path := range []string{component, filepath.Join("components", "ansible", "demo")} {
		info, err := ExtractComponentInfoFromPath(config, path)
		require.NoError(t, err)
		assert.Equal(t, "ansible", info.ComponentType)
		assert.Equal(t, "demo", info.FullComponent)
	}
	t.Chdir(component)
	info, err := ExtractComponentInfoFromPath(config, ".")
	require.NoError(t, err)
	assert.Equal(t, "ansible", info.ComponentType)
	assert.Equal(t, "demo", info.FullComponent)
}
