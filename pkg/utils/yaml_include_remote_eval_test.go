package utils

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRemoteYAMLIncludeEvaluation(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "child.txt"), []byte("outer-manifest-child"), 0o600))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := "home: !env HOME\nchild: !include ./child.txt\n"
		if r.URL.Path == "/invalid.yaml" {
			content = "value: [\n"
		}
		_, err := fmt.Fprint(w, content)
		assert.NoError(t, err)
	}))
	defer server.Close()
	config := &schema.AtmosConfiguration{BasePath: dir}
	for _, eval := range []bool{false, true} {
		t.Run(fmt.Sprintf("eval=%t", eval), func(t *testing.T) {
			option := ""
			if eval {
				option = " | eval"
			}
			input := "included: !include " + server.URL + "/config.yaml" + option + "\n"
			got, err := UnmarshalYAMLFromFile[map[string]any](config, input, filepath.Join(dir, "stack.yaml"))
			require.NoError(t, err)
			want := map[string]any{"home": "HOME", "child": "./child.txt"}
			if eval {
				want = map[string]any{"home": "!env HOME", "child": "outer-manifest-child"}
			}
			assert.Equal(t, map[string]any{"included": want}, got)
		})
	}
	_, err := UnmarshalYAMLFromFile[map[string]any](config, "included: !include "+server.URL+"/invalid.yaml | eval\n", filepath.Join(dir, "stack.yaml"))
	require.ErrorIs(t, err, ErrIncludeYamlFunctionFailedStackManifest)
}
