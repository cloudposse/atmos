package component

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestConfiguredLocalSourceSubdirSelectsModule(t *testing.T) {
	root := t.TempDir()
	module := filepath.Join(root, "modules", "app")
	require.NoError(t, os.MkdirAll(module, 0o755))
	info := &schema.ConfigAndStacksInfo{BaseComponentPath: "identity-only", ComponentSection: map[string]any{
		"metadata": map[string]any{"source_subdir": "modules/app"},
	}}
	selected, err := ResolveConfiguredSubpath(info, root)
	require.NoError(t, err)
	require.Equal(t, module, selected)
	info.ComponentSection[workdir.WorkdirPathKey] = root
	selected, err = ApplyWorkdirSubpathToSection(info)
	require.NoError(t, err)
	require.Equal(t, module, selected)
	require.NoError(t, os.RemoveAll(module))
	_, err = ResolveConfiguredSubpath(info, root)
	require.Error(t, err, "an explicit module path cannot fall back to the root")
}
