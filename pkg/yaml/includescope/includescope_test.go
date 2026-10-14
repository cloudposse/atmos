package includescope

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/function/parser"
)

// project builds base/scripts/a.star, base/stacks/workflows/local.star and a manifest path.
func project(t *testing.T) (base, manifest string) {
	t.Helper()
	base = t.TempDir()
	for _, file := range []string{
		filepath.Join("scripts", "a.star"),
		filepath.Join("stacks", "workflows", "local.star"),
		filepath.Join("stacks", "shared.star"),
	} {
		require.NoError(t, os.MkdirAll(filepath.Join(base, filepath.Dir(file)), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(base, file), []byte("x = 1\n"), 0o600))
	}
	return base, filepath.Join(base, "stacks", "workflows", "wf.yaml")
}

func TestScopeResolve(t *testing.T) {
	base, manifest := project(t)
	scope := Scope{File: manifest, BasePath: base}
	elsewhere := t.TempDir()
	// A file in the working directory with the bare name must never win over the base path.
	require.NoError(t, os.MkdirAll(filepath.Join(elsewhere, "scripts"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(elsewhere, "scripts", "a.star"), []byte("decoy"), 0o600))
	t.Chdir(elsewhere)

	absolute := filepath.Join(base, "scripts", "a.star")
	tests := []struct {
		name      string
		value     string
		wantLocal string
		wantQuery bool
		unchanged bool
	}{
		{name: "bare resolves against the base path", value: "scripts/a.star", wantLocal: absolute},
		{name: "dot resolves against the manifest directory", value: "./local.star", wantLocal: filepath.Join(base, "stacks", "workflows", "local.star")},
		{name: "dot dot resolves against the manifest directory", value: "../shared.star", wantLocal: filepath.Join(base, "stacks", "shared.star")},
		{name: "absolute is kept", value: absolute, wantLocal: absolute},
		{name: "quoted absolute is kept", value: `"` + absolute + `"`, wantLocal: absolute},
		{name: "missing bare path stays local so the include reports it", value: "scripts/missing.star", wantLocal: filepath.Join(base, "scripts", "missing.star")},
		{name: "query is preserved", value: "scripts/a.star .x", wantLocal: absolute, wantQuery: true},
		{name: "url is untouched", value: "https://example.com/a.star", unchanged: true},
		{name: "go-getter shorthand is untouched", value: "github.com/org/repo/a.yaml", unchanged: true},
		{name: "unparsable value is untouched", value: `"unterminated`, unchanged: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolved := scope.Resolve(tc.value)
			if tc.unchanged {
				assert.Equal(t, tc.value, resolved.Value)
				assert.Empty(t, resolved.LocalPath)
				return
			}
			assert.Equal(t, tc.wantLocal, resolved.LocalPath)
			assert.Equal(t, tc.wantQuery, resolved.HasQuery)
			parsed, err := parser.ParseInclude(resolved.Value)
			require.NoError(t, err)
			assert.Equal(t, tc.wantLocal, parsed.Path)
			if tc.wantQuery {
				assert.Equal(t, ".x", parsed.Query)
			}
		})
	}
}

func TestScopeResolveIsIndependentOfWorkingDirectory(t *testing.T) {
	base, manifest := project(t)
	scope := Scope{File: manifest, BasePath: base}
	first := scope.Resolve("scripts/a.star")

	t.Chdir(t.TempDir())
	assert.Equal(t, first, scope.Resolve("scripts/a.star"))
}

func TestScopeResolveKeepsAQueryThatStartsWithAQuote(t *testing.T) {
	base, manifest := project(t)
	resolved := Scope{File: manifest, BasePath: base}.Resolve(`scripts/a.star '"x"'`)
	parsed, err := parser.ParseInclude(resolved.Value)
	require.NoError(t, err)
	assert.Equal(t, `"x"`, parsed.Query)
}

func TestScopeRewriteAndLocalFile(t *testing.T) {
	base, manifest := project(t)
	var root yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(`
script: !include scripts/a.star
raw: !include.raw ./local.star
queried: !include scripts/a.star .x
remote: !include https://example.com/a.star
plain: scripts/a.star
nested:
  - script: !include.raw scripts/a.star
`), &root))
	Scope{File: manifest, BasePath: base}.Rewrite(&root)

	values := map[string]*yaml.Node{}
	mapping := root.Content[0]
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		values[mapping.Content[i].Value] = mapping.Content[i+1]
	}

	file, ok := LocalFile(values["script"])
	assert.True(t, ok)
	assert.Equal(t, filepath.Join(base, "scripts", "a.star"), file)

	file, ok = LocalFile(values["raw"])
	assert.True(t, ok)
	assert.Equal(t, filepath.Join(base, "stacks", "workflows", "local.star"), file)

	_, ok = LocalFile(values["queried"])
	assert.False(t, ok, "a YQ query changes the content, so the file is not the script source")
	_, ok = LocalFile(values["remote"])
	assert.False(t, ok)
	_, ok = LocalFile(values["plain"])
	assert.False(t, ok)

	nested, ok := LocalFile(values["nested"].Content[0].Content[1])
	assert.True(t, ok)
	assert.Equal(t, filepath.Join(base, "scripts", "a.star"), nested)

	Scope{}.Rewrite(nil)
	_, ok = LocalFile(nil)
	assert.False(t, ok)
}
