package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

// includeFromManifest processes a manifest at stacks/deploy/dev.yaml under a temporary base_path
// containing `value: <tag> <path>`, with optional files created relative to base_path first.
func includeFromManifest(t *testing.T, tag, path string, files map[string]string) (string, any, error) {
	t.Helper()

	basePath := t.TempDir()
	for name, content := range files {
		full := filepath.Join(basePath, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	}
	manifest := filepath.Join(basePath, "stacks", "deploy", "dev.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(manifest), 0o755))
	content := "value: " + tag + " " + path + "\n"
	require.NoError(t, os.WriteFile(manifest, []byte(content), 0o600))

	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(content), &node))
	err := processCustomTags(&schema.AtmosConfiguration{BasePath: basePath, BasePathAbsolute: basePath}, &node, manifest)
	var decoded map[string]any
	if err == nil {
		require.NoError(t, node.Decode(&decoded))
	}
	return basePath, decoded["value"], err
}

func TestIncludeNotFoundErrorsDescribeWhereEachPathKindWasResolved(t *testing.T) {
	tests := []struct {
		name         string
		tag          string
		path         string
		suffix       string
		files        map[string]string
		wantMessage  []string
		avoidMessage []string
		wantHint     string
		avoidHint    string
	}{
		{
			name:         "dot path suggests the base_path form when that file exists",
			tag:          AtmosYamlFuncInclude,
			path:         "./config/data.yaml",
			files:        map[string]string{"config/data.yaml": "src: base_path\n"},
			wantMessage:  []string{"could not find local file './config/data.yaml'", "relative to the including file"},
			avoidMessage: []string{"base path '"},
			wantHint:     "Did you mean `!include config/data.yaml`?",
		},
		{
			name:         "dot path without a base_path file gets no suggestion",
			tag:          AtmosYamlFuncInclude,
			path:         "./missing.yaml",
			wantMessage:  []string{"relative to the including file"},
			avoidMessage: []string{"base path '"},
			wantHint:     "relative to the file that contains the `!include`",
			avoidHint:    "Did you mean",
		},
		{
			name:         "parent path gets no base_path suggestion",
			tag:          AtmosYamlFuncInclude,
			path:         "../data.yaml",
			files:        map[string]string{"data.yaml": "src: base_path\n"},
			wantMessage:  []string{"relative to the including file"},
			avoidMessage: []string{"base path '"},
			avoidHint:    "Did you mean",
		},
		{
			name:        "suggestion keeps the query and eval option as written",
			tag:         AtmosYamlFuncInclude,
			path:        "./config/data.yaml",
			suffix:      " '.vars[\"a b\"]' | eval",
			files:       map[string]string{"config/data.yaml": "src: base_path\n"},
			wantMessage: []string{"relative to the including file"},
			wantHint:    "Did you mean `!include config/data.yaml '.vars[\"a b\"]' | eval`?",
		},
		{
			name:        "raw include suggestion uses its own tag",
			tag:         AtmosYamlFuncIncludeRaw,
			path:        "./config/script.sh",
			files:       map[string]string{"config/script.sh": "echo hi\n"},
			wantMessage: []string{"relative to the including file"},
			wantHint:    "Did you mean `!include.raw config/script.sh`?",
		},
		{
			name:         "bare path reports base_path and the working directory, not the manifest",
			tag:          AtmosYamlFuncInclude,
			path:         "config/missing.yaml",
			wantMessage:  []string{"tried relative to base path", "the working directory"},
			avoidMessage: []string{"manifest", "including file"},
			wantHint:     "Paths without a `./` or `../` prefix are relative to `base_path`",
			avoidHint:    "Did you mean",
		},
		{
			name:         "absolute path reports only the requested path",
			tag:          AtmosYamlFuncInclude,
			path:         filepath.Join(t.TempDir(), "missing.yaml"),
			wantMessage:  []string{"could not find local file"},
			avoidMessage: []string{"base path '", "including file"},
			avoidHint:    "relative to",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			basePath, _, err := includeFromManifest(t, tc.tag, tc.path+tc.suffix, tc.files)

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrIncludeYamlFunctionInvalidFile)
			for _, want := range tc.wantMessage {
				assert.Contains(t, err.Error(), want)
			}
			for _, avoid := range tc.avoidMessage {
				assert.NotContains(t, err.Error(), avoid)
			}
			hints := strings.Join(cockroachErrors.GetAllHints(err), "\n")
			if tc.wantHint != "" {
				assert.Contains(t, hints, tc.wantHint)
			}
			if tc.avoidHint != "" {
				assert.NotContains(t, hints, tc.avoidHint)
			}
			switch {
			case filepath.IsAbs(tc.path):
				assert.Contains(t, err.Error(), tc.path, "the error names the requested absolute path")
			case strings.HasPrefix(tc.path, "./"):
				resolved := filepath.Join(basePath, "stacks", "deploy", filepath.FromSlash(strings.TrimPrefix(tc.path, "./")))
				assert.Contains(t, err.Error(), resolved, "the error names the exact path that was tried")
			}
		})
	}
}

func TestReplaceIncludePathKeepsQuotingAndOptions(t *testing.T) {
	tests := []struct{ args, include, path, want string }{
		{"./data.yaml", "./data.yaml", "data.yaml", "data.yaml"},
		{"  ./data.yaml .vars | eval ", "./data.yaml", "data.yaml", "data.yaml .vars | eval"},
		{`"./my data.yaml" ".a b"`, "./my data.yaml", "my data.yaml", `"my data.yaml" ".a b"`},
		{`'./data.yaml' .vars`, "./data.yaml", "data.yaml", `'data.yaml' .vars`},
		// A path not written as its parsed form falls back to the bare suggestion.
		{"./other.yaml .vars", "./data.yaml", "data.yaml", "data.yaml"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, replaceIncludePath(tc.args, tc.include, tc.path), tc.args)
	}
}

// The suggestion is only advice: a `./` path that exists next to its manifest resolves there,
// even when base_path has a same-named file.
func TestIncludeDotPathResolvesNextToTheIncludingFile(t *testing.T) {
	_, value, err := includeFromManifest(t, AtmosYamlFuncInclude, "./data.yaml", map[string]string{
		"stacks/deploy/data.yaml": "src: next-to-manifest\n",
		"data.yaml":               "src: base_path\n",
	})

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"src": "next-to-manifest"}, value)
}
