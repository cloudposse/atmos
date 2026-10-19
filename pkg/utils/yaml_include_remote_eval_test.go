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

	errUtils "github.com/cloudposse/atmos/errors"
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

// A function inside remote YAML content is admitted as remote, not as the local manifest that included
// it. Attributing it to the parent would let a rule that denies remote functions admit them.
func TestRemoteYAMLIncludeFunctionsAreAdmittedAsRemote(t *testing.T) {
	dir := t.TempDir()
	policyDir := filepath.Join(dir, "policy.d")
	require.NoError(t, os.Mkdir(policyDir, 0o700))
	t.Setenv("ATMOS_POLICY_DIR", policyDir)
	manifest := "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: remote-literal}\nspec:\n  policies:\n    guard:\n      on: function\n      action: deny\n" +
		"      rule: input.name == 'literal' && input.provenance == 'remote'\n"
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, "policy.yaml"), []byte(manifest), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "local.yaml"), []byte("home: !literal HOME\n"), 0o600))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := fmt.Fprint(w, "home: !literal HOME\n")
		assert.NoError(t, err)
	}))
	defer server.Close()
	config := &schema.AtmosConfiguration{BasePath: dir}
	stack := filepath.Join(dir, "stack.yaml")

	_, err := UnmarshalYAMLFromFile[map[string]any](config, "included: !include "+server.URL+"/config.yaml | eval\n", stack)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied, "a denied function in remote content is denied")

	got, err := UnmarshalYAMLFromFile[map[string]any](config, "included: !include local.yaml | eval\n", stack)
	require.NoError(t, err, "the same function in a local include is not remote")
	assert.Equal(t, map[string]any{"included": map[string]any{"home": "HOME"}}, got)
}
