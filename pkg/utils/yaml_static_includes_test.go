package utils

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStaticIncludesKeepDecoderCachesSeparate(t *testing.T) {
	root := t.TempDir()
	include := filepath.Join(root, "included.yaml")
	require.NoError(t, os.WriteFile(include, []byte("value: resolved\n"), 0o600))
	source := fmt.Sprintf("value: !include %s\nraw: !include.raw %s\n", filepath.ToSlash(include), filepath.ToSlash(include))
	manifest := filepath.Join(root, "stack.yaml")
	regular := &schema.AtmosConfiguration{PreserveStackLiterals: true}
	static := &schema.AtmosConfiguration{PreserveStackLiterals: true, DeferStackIncludes: true}
	for _, config := range []*schema.AtmosConfiguration{regular, static, regular, static} {
		result, _, err := UnmarshalYAMLFromFileWithPositions[map[string]any](config, source, manifest)
		require.NoError(t, err)
		if config.DeferStackIncludes {
			require.Equal(t, "!include "+filepath.ToSlash(include), yamlsource.Expression(result["value"].(string)))
			require.Equal(t, "!include.raw "+filepath.ToSlash(include), yamlsource.Expression(result["raw"].(string)))
		} else {
			require.Equal(t, map[string]any{"value": "resolved"}, result["value"])
			require.Equal(t, "value: resolved\n", result["raw"])
		}
	}
}

func TestStaticIncludesNeverFetch(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("value: fetched\n"))
	}))
	t.Cleanup(server.Close)
	config := &schema.AtmosConfiguration{DeferStackIncludes: true}
	source := fmt.Sprintf("value: !include %s\nraw: !include.raw %s\n", server.URL, server.URL)
	result, err := UnmarshalYAMLFromFile[map[string]any](config, source, filepath.Join(t.TempDir(), "stack.yaml"))
	require.NoError(t, err)
	require.Equal(t, "!include "+server.URL, yamlsource.Expression(result["value"].(string)))
	require.Equal(t, "!include.raw "+server.URL, yamlsource.Expression(result["raw"].(string)))
	require.Zero(t, requests.Load())
}
