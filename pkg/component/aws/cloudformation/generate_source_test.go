package cloudformation

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedNestedSource(t *testing.T) {
	var archive bytes.Buffer
	compressed := gzip.NewWriter(&archive)
	writer := tar.NewWriter(compressed)
	const original = "Resources: {}\nDescription: original source\n"
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: "modules/service/original.yaml", Mode: 0o600, Size: int64(len(original))}))
	_, err := io.WriteString(writer, original)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, compressed.Close())
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "test" || password != "synthetic" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		requests.Add(1)
		_, _ = w.Write(archive.Bytes())
	}))
	defer server.Close()
	root := t.TempDir()
	for range 2 {
		config, info := generationFixture(t, root, "source")
		info.BaseComponentPath = "modules/service"
		info.ComponentSection["metadata"] = map[string]any{"component": "modules/service"}
		info.ComponentSection["source"] = map[string]any{"uri": strings.Replace(server.URL, "http://", "http://test:synthetic@", 1) + "/source.tar.gz", "ttl": "1h"}
		spec, err := resolveSpecAndTemplate(t.Context(), config, info, OperationApply)
		require.NoError(t, err)
		assert.Contains(t, spec.TemplateAbsPath, filepath.Join("modules", "service", "template.yaml"))
		assert.Contains(t, spec.TemplateBody, "Value: 'source'")
		contents, err := os.ReadFile(filepath.Join(filepath.Dir(spec.TemplateAbsPath), "original.yaml"))
		require.NoError(t, err)
		assert.Equal(t, original, string(contents))
	}
	assert.Positive(t, requests.Load())
	_, err = os.Stat(filepath.Join(root, "components"))
	assert.True(t, os.IsNotExist(err), "source and generated files belong only in the isolated workdir")
}

func TestGenerationDisabledByDefault(t *testing.T) {
	assert.False(t, DefaultConfig().AutoGenerateFiles)
	config, info := generationFixture(t, t.TempDir(), "disabled")
	config.Components.CloudFormation.AutoGenerateFiles = false
	info.ComponentSection["template"] = map[string]any{"Resources": map[string]any{}}
	delete(info.ComponentSection, "path")
	delete(info.ComponentSection, "stack_policy")
	spec, err := resolveSpecAndTemplate(t.Context(), config, info, OperationRender)
	require.NoError(t, err)
	assert.Contains(t, spec.TemplateBody, "Resources")
	entries, err := os.ReadDir(config.BasePath)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
