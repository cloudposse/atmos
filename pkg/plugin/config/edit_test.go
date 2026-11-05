package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/config"
)

var packagePath = []string{"plugins", "packages", "one"}

func fields(source, version string) []config.Field {
	return []config.Field{{Key: "source", Value: source}, {Key: "version", Value: version}}
}

func TestSetFieldsGolden(t *testing.T) {
	cases := []struct{ name, before, after string }{
		{"empty file", "", "plugins:\n  packages:\n    one:\n      source: oci://r/a\n      version: ^1\n"},
		{"comments only", "# only a comment\n", "# only a comment\nplugins:\n  packages:\n    one:\n      source: oci://r/a\n      version: ^1\n"},
		{"no trailing newline", "a: 1", "a: 1\nplugins:\n  packages:\n    one:\n      source: oci://r/a\n      version: ^1"},
		{"four space indent", "x:\n    y: 1\n", "x:\n    y: 1\nplugins:\n    packages:\n        one:\n            source: oci://r/a\n            version: ^1\n"},
		{"replace plain keeping inline comment", "plugins:\n  packages:\n    one:\n      source: oci://r/a   # why\n      version: ~1.0  # pin\n\nz: 1\n", "plugins:\n  packages:\n    one:\n      source: oci://r/a   # why\n      version: ^1  # pin\n\nz: 1\n"},
		{"replace quoted", "plugins:\n  packages:\n    one:\n      source: \"oci://r/old\"\n      version: '~1.0'\n", "plugins:\n  packages:\n    one:\n      source: oci://r/a\n      version: ^1\n"},
		{"keeps unrelated sibling fields", "plugins:\n  packages:\n    one:\n      config:\n        k: v\n      source: oci://r/a\n", "plugins:\n  packages:\n    one:\n      config:\n        k: v\n      source: oci://r/a\n      version: ^1\n"},
		{"crlf", "a: 1\r\n", "a: 1\r\nplugins:\r\n  packages:\r\n    one:\r\n      source: oci://r/a\r\n      version: ^1\r\n"},
		{"empty flow parent", "plugins:\n  packages: {}   # none\n", "plugins:\n  packages:   # none\n    one:\n      source: oci://r/a\n      version: ^1\n"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			after, err := config.SetFields([]byte(testCase.before), packagePath, fields("oci://r/a", "^1"))
			require.NoError(t, err)
			assert.Equal(t, testCase.after, string(after))
			var parsed map[string]any
			require.NoError(t, yaml.Unmarshal(after, &parsed))
		})
	}
}

func TestSetFieldsWritesBlockSequences(t *testing.T) {
	after, err := config.SetFields([]byte("# c\n"), []string{"plugins", "registries", "acme"}, []config.Field{{Key: "catalog", Value: "oci://r/c:stable"}, {Key: "repositories", Items: []string{"oci://r/a", "oci://r/b"}}})
	require.NoError(t, err)
	assert.Equal(t, "# c\nplugins:\n  registries:\n    acme:\n      catalog: oci://r/c:stable\n      repositories:\n        - oci://r/a\n        - oci://r/b\n", string(after))
	replaced, err := config.SetFields(after, []string{"plugins", "registries", "acme"}, []config.Field{{Key: "repositories", Items: []string{"oci://r/z"}}})
	require.NoError(t, err)
	assert.Equal(t, "# c\nplugins:\n  registries:\n    acme:\n      catalog: oci://r/c:stable\n      repositories:\n        - oci://r/z\n", string(replaced))
}

func TestSetFieldsFlowLayoutFallsBackAndStaysCorrect(t *testing.T) {
	after, err := config.SetFields([]byte("plugins: {packages: {one: {source: x, version: y}}}\n"), packagePath, fields("oci://r/a", "^1"))
	require.NoError(t, err)
	var parsed struct {
		Plugins struct {
			Packages map[string]struct{ Source, Version string }
		}
	}
	require.NoError(t, yaml.Unmarshal(after, &parsed))
	assert.Equal(t, "^1", parsed.Plugins.Packages["one"].Version)
}

func TestDeleteKeyGolden(t *testing.T) {
	before := "# top\nplugins:\n  packages:\n    # about one\n    one:\n      source: a\n      version: b\n    two:\n      source: c   # keep\n\nz: 1\n"
	after, err := config.DeleteKey([]byte(before), packagePath)
	require.NoError(t, err)
	assert.Equal(t, "# top\nplugins:\n  packages:\n    two:\n      source: c   # keep\n\nz: 1\n", string(after))
	missing, err := config.DeleteKey(after, packagePath)
	require.NoError(t, err)
	assert.Equal(t, string(after), string(missing), "deleting an absent key is a no-op")
	last, err := config.DeleteKey(after, []string{"plugins", "packages", "two"})
	require.NoError(t, err)
	assert.Equal(t, "# top\nplugins:\n  packages: {}\n\nz: 1\n", string(last))
}

func TestDeleteKeyRejectsEmptyPath(t *testing.T) {
	for name, path := range map[string][]string{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			result, err := config.DeleteKey([]byte("plugins: {}\n"), path)
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			require.Nil(t, result)
		})
	}
}

func TestSetFieldsPreservesQuotedTokenComments(t *testing.T) {
	for name, value := range map[string]string{
		"escaped double quote": `"old\"value"`,
		"escaped single quote": `'old''value'`,
		"plain hash":           "old#value",
		"multiline quoted":     "\"old\n        value\"",
	} {
		t.Run(name, func(t *testing.T) {
			before := "plugins:\n  packages:\n    one:\n      source: " + value + "   # preserve\n      version: ^1\n"
			after, err := config.SetFields([]byte(before), packagePath, []config.Field{{Key: "source", Value: "oci://r/new"}})
			require.NoError(t, err)
			assert.Contains(t, string(after), "# preserve")
			var parsed struct {
				Plugins struct {
					Packages map[string]struct{ Source, Version string }
				}
			}
			require.NoError(t, yaml.Unmarshal(after, &parsed))
			assert.Equal(t, "oci://r/new", parsed.Plugins.Packages["one"].Source)
			assert.Equal(t, "^1", parsed.Plugins.Packages["one"].Version)
		})
	}
}

func TestSetFieldsFallbackPreservesValues(t *testing.T) {
	for name, before := range map[string]string{
		"root flow":            "{plugins: {packages: {one: {version: old}}}}\n",
		"root anchor":          "&root\nplugins:\n  packages:\n    one:\n      version: old\n",
		"anchored target":      "plugins:\n  packages:\n    one: &target\n      version: old\n",
		"block scalar sibling": "plugins:\n  packages:\n    one:\n      description: |\n        preserved\n",
		"flow sibling":         "plugins:\n  packages:\n    one:\n      metadata: {keep: value}\n",
		"empty spaced mapping": "plugins:\n  packages:\n    one: { }\n",
		"explicit null":        "null\n",
	} {
		t.Run(name, func(t *testing.T) {
			after, err := config.SetFields([]byte(before), packagePath, []config.Field{
				{Key: "source", Value: "oci://r/new"},
				{Key: "description", Value: "line one\nline two"},
				{Key: "repositories", Items: []string{"oci://r/first", "line one\nline two"}},
			})
			require.NoError(t, err)
			var parsed struct {
				Plugins struct {
					Packages map[string]struct {
						Source, Description string
						Repositories        []string
					}
				}
			}
			require.NoError(t, yaml.Unmarshal(after, &parsed))
			actual := parsed.Plugins.Packages["one"]
			assert.Equal(t, "oci://r/new", actual.Source)
			assert.Equal(t, "line one\nline two", actual.Description)
			assert.Equal(t, []string{"oci://r/first", "line one\nline two"}, actual.Repositories)
		})
	}
}

func TestDeleteKeyFallbackAndMissingParents(t *testing.T) {
	for name, before := range map[string]string{
		"empty document": "",
		"missing parent": "unrelated: value\n",
		"empty parent":   "plugins:\n  packages:\n",
		"flow target":    "plugins: {packages: {one: {source: old}, two: {source: keep}}}\n",
		"block scalar":   "plugins:\n  packages:\n    one: |\n      remove this\n    two:\n      source: keep\n",
		"quoted parent":  "plugins:\n  'packages':\n    one:\n      source: old\n",
	} {
		t.Run(name, func(t *testing.T) {
			after, err := config.DeleteKey([]byte(before), packagePath)
			require.NoError(t, err)
			var parsed struct {
				Plugins struct{ Packages map[string]any }
			}
			require.NoError(t, yaml.Unmarshal(after, &parsed))
			assert.NotContains(t, parsed.Plugins.Packages, "one")
			if name == "flow target" || name == "block scalar" {
				assert.Equal(t, map[string]any{"source": "keep"}, parsed.Plugins.Packages["two"])
			}
		})
	}
}

func TestSetFieldsRejectsMalformedYAML(t *testing.T) {
	_, err := config.SetFields([]byte("plugins: [unterminated"), packagePath, fields("oci://r/a", "^1"))
	require.Error(t, err)
	_, err = config.DeleteKey([]byte("plugins: [unterminated"), packagePath)
	require.Error(t, err)
}
