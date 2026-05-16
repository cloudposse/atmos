package verification

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
)

func TestRenderTemplateStringAquaFieldsAndFunctions(t *testing.T) {
	rendered, err := renderTemplateString(
		"{{.RepoOwner}}/{{.RepoName}}/{{trimV .Version}}/{{trimPrefix \"tool-\" .AssetWithoutExt}}/{{replace \"_\" \"-\" .OS}}/{{.Format}}",
		&registry.Tool{
			RepoOwner:     "owner",
			RepoName:      "tool",
			VersionPrefix: "v",
			Format:        "tar.gz",
			Replacements:  map[string]string{"darwin": "darwin_os"},
		},
		"1.2.3",
		"tool-v1.2.3.tar.gz",
		map[string]string{"darwin_os": "darwin"},
	)

	require.NoError(t, err)
	assert.Contains(t, rendered, "owner/tool/1.2.3/v1.2.3/darwin-os/tar.gz")
}

func TestRenderTemplateStringErrors(t *testing.T) {
	_, err := renderTemplateString("{{", &registry.Tool{}, "1.0.0", "tool.tar.gz", nil)
	require.Error(t, err)

	_, err = renderTemplateString("{{.Missing.Field}}", &registry.Tool{}, "1.0.0", "tool.tar.gz", nil)
	require.Error(t, err)
}

func TestStripFileExtensionVariants(t *testing.T) {
	assert.Equal(t, "tool", stripFileExtension("tool.tar.xz"))
	assert.Equal(t, "tool", stripFileExtension("tool.zip"))
	assert.Equal(t, "tool", stripFileExtension("tool"))
}
