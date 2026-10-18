package docs

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestGenerateInvalidMetadata preserves existing output when input validation fails.
func TestGenerateInvalidMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	invalidYAML := filepath.Join(dir, "invalid.yaml")
	require.NoError(t, os.WriteFile(invalidYAML, []byte("name: ["), 0o600))
	for _, tc := range []struct {
		name  string
		input any
	}{
		{"unsupported type", 42},
		{"invalid YAML", invalidYAML},
		{"empty path", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			output := filepath.Join(t.TempDir(), "README.md")
			require.NoError(t, os.WriteFile(output, []byte("existing README"), 0o600))
			settings := &schema.DocsGenerate{Input: []any{tc.input}, Output: output}
			err := Generate(&schema.AtmosConfiguration{}, dir, settings)
			require.ErrorIs(t, err, errUtils.ErrMergeInputYAMLs)
			if tc.name == "unsupported type" {
				assert.ErrorIs(t, err, errUtils.ErrUnsupportedInputType)
			}
			assert.Equal(t, "existing README", readDocsTestFile(t, output))
		})
	}
}

// TestGenerateUnwritableDestination reports write failures without removing the destination.
func TestGenerateUnwritableDestination(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	output := filepath.Join(dir, "README.md")
	require.NoError(t, os.Mkdir(output, 0o700))
	err := Generate(&schema.AtmosConfiguration{}, dir, &schema.DocsGenerate{Output: output})
	require.ErrorIs(t, err, errUtils.ErrWriteOutput)
	assert.ErrorContains(t, err, output)
	info, err := os.Stat(output)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestDocsSourceErrors covers missing paths and failed downloads without external services.
func TestDocsSourceErrors(t *testing.T) {
	dir := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, dir)
	}
	cfg := &schema.AtmosConfiguration{}
	_, err := getTemplateContent(cfg, "", dir)
	require.ErrorIs(t, err, errUtils.ErrPathResolution)
	_, _, err = downloadSource(cfg, "", dir)
	require.ErrorIs(t, err, errUtils.ErrPathResolution)
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	_, err = getTemplateContent(cfg, srv.URL+"/missing.gomplate", dir)
	require.ErrorIs(t, err, errUtils.ErrDownloadPackage)
	_, err = fetchAndParseYAML(cfg, srv.URL+"/missing.yaml", dir)
	require.ErrorIs(t, err, errUtils.ErrDownloadPackage)
}

// TestDownloadSourceMissingTempDirectory retains the temp-directory sentinel and filesystem cause.
func TestDownloadSourceMissingTempDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, missing)
	}
	_, _, err := downloadSource(&schema.AtmosConfiguration{}, "https://example.com/unused.yaml", "")
	require.ErrorIs(t, err, errUtils.ErrCreateTempDir)
	assert.ErrorContains(t, err, missing)
}
