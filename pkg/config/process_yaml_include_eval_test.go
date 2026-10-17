package config

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestDecodeConfigIncludeEvaluation(t *testing.T) {
	t.Setenv("ATMOS_INCLUDE_EVAL_TEST", "resolved-home")
	dir := t.TempDir()
	sourceFile := filepath.Join(dir, "atmos.yaml")
	content := "home: !env ATMOS_INCLUDE_EVAL_TEST\nnested: !include ./child.yaml | eval\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "child.yaml"), []byte("value: !env ATMOS_INCLUDE_EVAL_TEST\n"), 0o600))
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(sub, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "config.yaml"), []byte(content), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(sub, "child.yaml"), []byte("value: local-child\n"), 0o600))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := fmt.Fprint(w, content)
		assert.NoError(t, err)
	}))
	defer server.Close()

	for _, target := range []struct{ name, path, child string }{
		{"remote", server.URL + "/config.yaml", "resolved-home"},
		{"local", filepath.Join(sub, "config.yaml"), "local-child"},
	} {
		for _, eval := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/eval=%t", target.name, eval), func(t *testing.T) {
				option := ""
				if eval {
					option = " | eval"
				}
				var node yaml.Node
				require.NoError(t, yaml.Unmarshal([]byte(fmt.Sprintf("included: !include %q\n", fmt.Sprintf("%q%s", target.path, option))), &node))
				got, err := decodeNodeWithYamlFunctionsForFile(&node, sourceFile)
				require.NoError(t, err)
				expected := map[string]any{"home": "ATMOS_INCLUDE_EVAL_TEST", "nested": "./child.yaml | eval"}
				if eval {
					expected = map[string]any{"home": "resolved-home", "nested": map[string]any{"value": target.child}}
				}
				assert.Equal(t, map[string]any{"included": expected}, got)
			})
		}
	}
}

func TestDecodeConfigIncludeEvaluationErrors(t *testing.T) {
	for _, tc := range []struct{ name, content, want string }{
		{"local cycle", "next: !include ./config.yaml | eval\n", "recursive include"},
		{"unsupported tag", "value: !terraform.output component output\n", "unsupported YAML tag"},
		{"invalid yaml", "value: [\n", "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(tc.content), 0o600))
			var node yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte("value: !include ./config.yaml | eval\n"), &node))
			_, err := decodeNodeWithYamlFunctionsForFile(&node, filepath.Join(dir, "atmos.yaml"))
			require.ErrorIs(t, err, ErrExecuteYamlFunctions)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestDecodeConfigRemoteIncludeCycle(t *testing.T) {
	var target string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := fmt.Fprintf(w, "next: !include %s | eval\n", target)
		assert.NoError(t, err)
	}))
	defer server.Close()
	target = server.URL + "/cycle.yaml"
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("value: !include "+target+" | eval\n"), &node))
	_, err := decodeNodeWithYamlFunctionsForFile(&node, filepath.Join(t.TempDir(), "atmos.yaml"))
	require.ErrorIs(t, err, ErrExecuteYamlFunctions)
	assert.Contains(t, err.Error(), "recursive include")
}

func TestDecodeConfigIncludeEvaluationRunsOnce(t *testing.T) {
	t.Setenv("ATMOS_INCLUDE_EVAL_TEST", "!env ATMOS_INCLUDE_EVAL_OTHER")
	t.Setenv("ATMOS_INCLUDE_EVAL_OTHER", "must-not-be-evaluated")
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("!env ATMOS_INCLUDE_EVAL_TEST\n"), 0o600))
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("first: !include ./config.yaml | eval\nsecond: !include ./config.yaml | eval\n"), &node))
	got, err := decodeNodeWithYamlFunctionsForFile(&node, filepath.Join(dir, "atmos.yaml"))
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"first": "!env ATMOS_INCLUDE_EVAL_OTHER", "second": "!env ATMOS_INCLUDE_EVAL_OTHER"}, got)
}

func TestDecodeConfigIncludeDepthLimit(t *testing.T) {
	dir := t.TempDir()
	for i := range maxConfigIncludeDepth + 1 {
		name := fmt.Sprintf("include-%d.yaml", i)
		content := fmt.Sprintf("next: !include ./include-%d.yaml | eval\n", i+1)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("value: !include ./include-0.yaml | eval\n"), &node))
	_, err := decodeNodeWithYamlFunctionsForFile(&node, filepath.Join(dir, "atmos.yaml"))
	require.ErrorIs(t, err, ErrExecuteYamlFunctions)
	assert.Contains(t, err.Error(), "recursive include")
}

func TestMergeConfigFileRemoteIncludeEvaluation(t *testing.T) {
	t.Setenv("ATMOS_INCLUDE_EVAL_TEST", "resolved-home")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/invalid.yaml" {
			_, err := fmt.Fprint(w, "value: [\n")
			assert.NoError(t, err)
			return
		}
		_, err := fmt.Fprint(w, "home: !env ATMOS_INCLUDE_EVAL_TEST\n")
		assert.NoError(t, err)
	}))
	defer server.Close()
	for _, tc := range []struct {
		name, suffix, option, want string
		invalid                    bool
	}{
		{name: "evaluated", suffix: "/env.yaml", option: " | eval", want: "resolved-home"},
		{name: "data only", suffix: "/env.yaml", want: "ATMOS_INCLUDE_EVAL_TEST"},
		{name: "invalid", suffix: "/invalid.yaml", option: " | eval", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "atmos.yaml")
			content := fmt.Sprintf("commands:\n  - name: example\n    env: !include %s%s%s\n", server.URL, tc.suffix, tc.option)
			require.NoError(t, os.WriteFile(file, []byte(content), 0o600))
			v := viper.New()
			v.SetConfigType("yaml")
			err := mergeConfigFile(file, v)
			if tc.invalid {
				require.ErrorIs(t, err, ErrExecuteYamlFunctions)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, []any{map[string]any{"name": "example", "env": map[string]any{"home": tc.want}}}, v.Get("commands"))
		})
	}
}
