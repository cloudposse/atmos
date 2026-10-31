package templates

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestLoaderStrictComponentTypes asserts that a configured override that does not exist is an error
// for the container and artifacts components, and still falls through for the plugin components.
func TestLoaderStrictComponentTypes(t *testing.T) {
	defaults := fstest.MapFS{"templates/summary.md": {Data: []byte("default")}}
	base := t.TempDir()

	cases := []struct {
		name          string
		componentType string
		templates     schema.CITemplatesConfig
		wantErr       bool
	}{
		{"artifacts missing override", "artifacts", schema.CITemplatesConfig{BasePath: base, Artifacts: map[string]string{"summary": "gone.md"}}, true},
		{"container missing override", "container", schema.CITemplatesConfig{BasePath: base, Container: map[string]string{"summary": "gone.md"}}, true},
		{"terraform missing override falls through", "terraform", schema.CITemplatesConfig{BasePath: base, Terraform: map[string]string{"summary": "gone.md"}}, false},
		{"artifacts key unset uses the default", "artifacts", schema.CITemplatesConfig{BasePath: base}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loader := NewLoader(&schema.AtmosConfiguration{CI: schema.CIConfig{Templates: tc.templates}})
			content, err := loader.Load(tc.componentType, "summary", defaults)
			if tc.wantErr {
				require.ErrorIs(t, err, errUtils.ErrCITemplateNotFound)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "default", content)
		})
	}
}

// TestLoaderArtifactsPrecedence asserts the explicit, convention, default order for artifacts.
func TestLoaderArtifactsPrecedence(t *testing.T) {
	defaults := fstest.MapFS{"templates/summary.md": {Data: []byte("default")}}
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "artifacts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "artifacts", "summary.md"), []byte("convention"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(base, "explicit.md"), []byte("explicit"), 0o644))

	config := func(artifacts map[string]string) *schema.AtmosConfiguration {
		return &schema.AtmosConfiguration{CI: schema.CIConfig{Templates: schema.CITemplatesConfig{BasePath: base, Artifacts: artifacts}}}
	}
	explicit, err := NewLoader(config(map[string]string{"summary": "explicit.md"})).Load("artifacts", "summary", defaults)
	require.NoError(t, err)
	assert.Equal(t, "explicit", explicit)

	convention, err := NewLoader(config(nil)).Load("artifacts", "summary", defaults)
	require.NoError(t, err)
	assert.Equal(t, "convention", convention)

	fallback, err := NewLoader(&schema.AtmosConfiguration{}).Load("artifacts", "summary", defaults)
	require.NoError(t, err)
	assert.Equal(t, "default", fallback)
}
