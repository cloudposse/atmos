package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValidatePathPackages(t *testing.T) {
	disabled := false
	cases := []struct {
		name    string
		pkg     schema.PluginPackage
		wantErr string
	}{
		{"path alone", schema.PluginPackage{Path: "build/corporate-oidc"}, ""},
		{"path beside source and version", schema.PluginPackage{Source: "oci://r/a", Version: "^1", Path: "build/corporate-oidc"}, ""},
		{"path beside source only", schema.PluginPackage{Source: "oci://r/a", Path: "build/x"}, ""},
		{"absolute path", schema.PluginPackage{Path: filepath.Join(t.TempDir(), "plugin")}, ""},
		{"neither path nor source", schema.PluginPackage{Version: "^1"}, "plugins.packages.corporate.source"},
		{"neither path nor version", schema.PluginPackage{Source: "oci://r/a"}, "plugins.packages.corporate.version"},
		{"blank path without source", schema.PluginPackage{Path: "   "}, "plugins.packages.corporate.path"},
		{"blank path with release", schema.PluginPackage{Source: "oci://r/a", Version: "^1", Path: "\t"}, "plugins.packages.corporate.path"},
		{"NUL in path", schema.PluginPackage{Path: "build/\x00evil"}, "plugins.packages.corporate.path"},
		{"path with bad timeout", schema.PluginPackage{Path: "build/x", Timeout: "soon"}, "plugins.packages.corporate.timeout"},
		{"disabled package may be incomplete", schema.PluginPackage{Enabled: &disabled, Path: " "}, ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := Validate(&schema.PluginsConfig{Packages: map[string]schema.PluginPackage{"corporate": testCase.pkg}})
			if testCase.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
			assert.Contains(t, err.Error(), testCase.wantErr)
		})
	}
}

func TestLocalSelectionsResolveRelativeToTheProject(t *testing.T) {
	project := t.TempDir()
	absolute := filepath.Join(t.TempDir(), "elsewhere")
	disabled := false
	packages := map[string]schema.PluginPackage{
		"relative": {Source: "oci://r/a", Version: "^1", Path: filepath.Join("build", "relative")},
		"absolute": {Path: absolute},
		"release":  {Source: "oci://r/b", Version: "^1"},
		"disabled": {Path: "build/off", Enabled: &disabled},
	}
	selections, err := LocalSelections(project, packages)
	require.NoError(t, err)
	require.Len(t, selections.Packages, 2, "release and disabled packages are not local")
	assert.Equal(t, Selection{Path: filepath.Join(project, "build", "relative"), Local: true}, selections.Packages["relative"])
	assert.Equal(t, Selection{Path: absolute, Local: true}, selections.Packages["absolute"])
	assert.True(t, IsLocal(&schema.PluginPackage{Path: "x"}))
	assert.False(t, IsLocal(&schema.PluginPackage{Source: "oci://r/b"}), "without a path the package behaves as before")
}

func TestDisplayPath(t *testing.T) {
	workingDirectory, err := filepath.Abs(".")
	require.NoError(t, err)
	for name, testCase := range map[string]struct{ input, expected string }{
		"relative":          {filepath.Join("build", "..", "plugin"), "plugin"},
		"working directory": {workingDirectory, "."},
		"descendant":        {filepath.Join(workingDirectory, "build", "plugin"), filepath.Join("build", "plugin")},
		"parent":            {filepath.Dir(workingDirectory), filepath.Dir(workingDirectory)},
		"sibling":           {filepath.Join(filepath.Dir(workingDirectory), "another-project", "plugin"), filepath.Join(filepath.Dir(workingDirectory), "another-project", "plugin")},
	} {
		t.Run(name, func(t *testing.T) { assert.Equal(t, testCase.expected, DisplayPath(testCase.input)) })
	}
}
