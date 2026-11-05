package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestCheckReleaseSourceRejectsPathsWithoutNetworkAccess(t *testing.T) {
	pluginDirectory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(pluginDirectory, "plugin.yaml"), []byte("kind: AtmosPluginConfig\n"), 0o644))
	layout := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(layout, "oci-layout"), []byte(`{"imageLayoutVersion":"1.0.0"}`), 0o644))
	paths := []string{"/abs/path/plugin", "./relative", "../sibling", "~/plugin", `C:\plugins\x`, `D:/plugins/x`, `dir\sub`, pluginDirectory, layout + "/", "."}
	for _, source := range paths {
		t.Run(source, func(t *testing.T) {
			err := CheckReleaseSource(source)
			require.ErrorIs(t, err, errUtils.ErrPluginSourceIsPath)
			hints := strings.Join(errUtils.AllHints(err), "\n")
			assert.Contains(t, hints, "declare `path: "+source+"`", "the hint tells the user to declare the source path for the package")
			assert.Contains(t, hints, "`plugins.packages.<alias>`")
			assert.NotContains(t, hints, "--dev")
		})
	}
	err := CheckReleaseSource(layout + "/")
	assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "--from-layout", "an OCI layout directory also names the offline flag")
	for _, source := range []string{"oci://ghcr.io/acme/plugin", "local/oidc", "acme/oidc", "oidc"} {
		require.NoError(t, CheckReleaseSource(source), source)
	}
}

func TestParseSelectorRejectsEmptySegmentsLocally(t *testing.T) {
	registry, name, err := ParseSelector("local/acme/oidc")
	require.NoError(t, err)
	assert.Equal(t, "local", registry)
	assert.Equal(t, "acme/oidc", name)
	for _, source := range []string{"", "oidc", "/oidc", "local/", "local//oidc", "local/acme/"} {
		_, _, err := ParseSelector(source)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig, source)
		assert.Contains(t, errUtils.Format(err, errUtils.DefaultFormatterConfig()), "registry", source)
	}
}
