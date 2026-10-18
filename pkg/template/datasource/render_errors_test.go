package datasource

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestRenderMetadataAndFunctions preserves metadata while scoping functions to each render.
func TestRenderMetadataAndFunctions(t *testing.T) {
	t.Parallel()
	data := map[string]any{"name": "example", "Env": map[string]any{"custom": "kept", "README_YAML": "original"}}
	for _, value := range []string{"first", "second"} {
		actual, err := Render(&schema.AtmosConfiguration{}, "readme", `{{ .name }}|{{ (ds "config").name }}|{{ .Env.custom }}|{{ value }}`, data,
			template.FuncMap{"value": func() string { return value }})
		require.NoError(t, err)
		assert.Equal(t, "example|example|kept|"+value, actual)
	}
	assert.Equal(t, "original", data["Env"].(map[string]any)["README_YAML"])
	actual, err := Render(&schema.AtmosConfiguration{}, "readme", `{{ value }}`, data, nil)
	require.ErrorContains(t, err, `function "value" not defined`)
	assert.Empty(t, actual)
}

// TestRenderCleanup verifies temporary datasource files are removed after success and failure.
func TestRenderCleanup(t *testing.T) {
	tempDir := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, tempDir)
	}
	for _, tc := range []struct {
		name, source, wantError string
		data                    map[string]any
	}{
		{"success", "rendered", "", nil},
		{"invalid metadata", "", "failed to marshal merged data", map[string]any{"invalid": make(chan int)}},
		{"invalid template", "{{", "failed to render template", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := Render(&schema.AtmosConfiguration{}, "readme", tc.source, tc.data, nil)
			if tc.wantError == "" {
				require.NoError(t, err)
				assert.Equal(t, "rendered", actual)
			} else {
				require.ErrorContains(t, err, tc.wantError)
				assert.Empty(t, actual)
			}
			entries, err := os.ReadDir(tempDir)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

// TestRenderMissingTempDirectory checks errors retain the underlying filesystem cause.
func TestRenderMissingTempDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, missing)
	}
	actual, err := Render(&schema.AtmosConfiguration{}, "readme", "unused", nil, nil)
	require.ErrorIs(t, err, os.ErrNotExist)
	assert.ErrorContains(t, err, "failed to create temp directory")
	assert.Empty(t, actual)
}

// TestDatasourceFileErrors covers invalid metadata and unavailable destinations for both files.
func TestDatasourceFileErrors(t *testing.T) {
	t.Parallel()
	writers := map[string]func(string, map[string]any) (*url.URL, error){
		"config": writeMergedDataToFile,
		"root": func(dir string, data map[string]any) (*url.URL, error) {
			return writeOuterTopLevelFile(dir, "file:///config.json", data)
		},
	}
	for name, write := range writers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			result, err := write(dir, map[string]any{"invalid": make(chan int)})
			var unsupported *json.UnsupportedTypeError
			require.ErrorAs(t, err, &unsupported)
			assert.Nil(t, result)
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Empty(t, entries)
			result, err = write(filepath.Join(dir, "missing"), map[string]any{"name": "example"})
			require.ErrorIs(t, err, os.ErrNotExist)
			assert.Nil(t, result)
		})
	}
}

// TestRenderWithBuildsFunctionsFromDatasourceURLs gives the builder the render's own datasource files.
func TestRenderWithBuildsFunctionsFromDatasourceURLs(t *testing.T) {
	// Not parallel: gomplate keeps a package-level cleanup hook that concurrent renderers race on.
	data := map[string]any{"name": "example"}

	var topLevel, config *url.URL
	actual, err := RenderWith(&schema.AtmosConfiguration{}, "readme", `{{ name }}|{{ (ds "config").name }}`, data,
		func(top, cfg *url.URL) (template.FuncMap, error) {
			topLevel, config = top, cfg
			return template.FuncMap{"name": func() string { return "built" }}, nil
		})
	require.NoError(t, err)
	assert.Equal(t, "built|example", actual)
	require.NotNil(t, topLevel)
	require.NotNil(t, config)
	assert.Equal(t, "file", topLevel.Scheme)
	assert.Equal(t, "file", config.Scheme)
	assert.NotEqual(t, topLevel.String(), config.String())
}

// TestRenderWithBuilderErrorStopsTheRender returns the builder's error before any template runs.
func TestRenderWithBuilderErrorStopsTheRender(t *testing.T) {
	t.Parallel()
	failure := assert.AnError
	actual, err := RenderWith(&schema.AtmosConfiguration{}, "readme", `unreached`, map[string]any{},
		func(*url.URL, *url.URL) (template.FuncMap, error) { return nil, failure })
	require.ErrorIs(t, err, failure)
	assert.Empty(t, actual)
}
