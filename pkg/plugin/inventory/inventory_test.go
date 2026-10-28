package inventory

import (
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

var (
	_ = File{Packages: nil, Runners: nil}
	_ = Package{Path: "", Repository: "", Inputs: nil}
)

func TestParsePreservesOptionalBuildMetadata(t *testing.T) {
	const packages = "packages:\n  example:\n    path: plugins/example\n    repository: oci://registry.example.com/example\n"
	file, err := Parse([]byte(packages))
	require.NoError(t, err)
	require.Equal(t, map[string]Package{"example": {Path: "plugins/example", Repository: "oci://registry.example.com/example"}}, file.Packages)
	require.Empty(t, file.Runners)
	file, err = Parse([]byte(packages + "    inputs: [sdk/**, go.mod]\nrunners:\n  linux/amd64: ubuntu-latest\n"))
	require.NoError(t, err)
	require.Equal(t, []string{"sdk/**", "go.mod"}, file.Packages["example"].Inputs)
	require.Equal(t, map[string]string{"linux/amd64": "ubuntu-latest"}, file.Runners)
	for _, raw := range []string{"[", "{}", packages + "unknown: true", packages + "---\n{}"} {
		_, err := Parse([]byte(raw))
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	}
}

func TestSafePath(t *testing.T) {
	for _, value := range []string{"", ".", "..", "../plugins", "/plugins", "plugins/../other", "plugins//example", "plugins\\example", "C:/plugins", "plugins\x00", "plugins\n", "plugins\r"} {
		require.False(t, SafePath(value), value)
	}
	for _, value := range []string{"plugins/example", "releases.yaml", "sdk/**"} {
		require.True(t, SafePath(value), value)
	}
}
