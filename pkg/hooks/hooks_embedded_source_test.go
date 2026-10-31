package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/delimiters"
)

func TestHookRendererResolvesEmbeddedSource(t *testing.T) {
	t.Setenv("ATMOS_HOOK_EMBED", "from-env")
	embedded := "x-" + yamlsource.Source{Tag: "!env", Argument: "ATMOS_HOOK_EMBED"}.Encode() + "-y"
	renderer := hookRenderer{atmosConfig: &schema.AtmosConfiguration{}, delims: delimiters.Default()}

	result, err := renderer.str(embedded, &schema.ConfigAndStacksInfo{Stack: "test"})

	require.NoError(t, err)
	assert.Equal(t, "x-from-env-y", result)
}

func TestHookRendererLeavesAuthoredStringLiteral(t *testing.T) {
	t.Setenv("ATMOS_HOOK_EMBED", "from-env")
	renderer := hookRenderer{atmosConfig: &schema.AtmosConfiguration{}, delims: delimiters.Default()}

	result, err := renderer.str("x-!env ATMOS_HOOK_EMBED", &schema.ConfigAndStacksInfo{Stack: "test"})

	require.NoError(t, err)
	assert.Equal(t, "x-!env ATMOS_HOOK_EMBED", result)
}

func TestHookRendererResolvesSourceProducedByRendering(t *testing.T) {
	t.Setenv("ATMOS_HOOK_EMBED", "from-env")
	source := yamlsource.Source{Tag: "!env", Argument: "ATMOS_HOOK_EMBED"}.Encode()
	info := &schema.ConfigAndStacksInfo{Stack: "test", ComponentSection: map[string]any{"vars": map[string]any{"a": source}}}
	renderer := hookRenderer{atmosConfig: &schema.AtmosConfiguration{}, delims: delimiters.Default()}

	embedded, err := renderer.str("echo x-{{ .vars.a }}", info)
	require.NoError(t, err)
	assert.Equal(t, "echo x-from-env", embedded)

	whole, err := renderer.str("{{ .vars.a }}", info)
	require.NoError(t, err)
	assert.Equal(t, "from-env", whole)
}
