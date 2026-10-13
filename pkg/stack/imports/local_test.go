package imports

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func writeFiles(t *testing.T, root string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("{}\n"), 0o644))
	}
}

func TestResolveLocal(t *testing.T) {
	root := t.TempDir()
	writeFiles(
		t, root,
		"orgs/acme/_defaults.yaml",
		"orgs/acme/dev.yaml",
		"catalog/vpc.yml",
		"catalog/tmpl-only.yaml.tmpl",
		"catalog/both.yaml",
		"catalog/both.yaml.tmpl",
		"catalog/ytmpl.yml.tmpl",
		"mixins/a.yaml",
		"mixins/deep/b.yaml",
		"mixins/c.yml",
	)
	importer := filepath.Join(root, "orgs", "acme", "dev.yaml")

	tests := []struct {
		name    string
		imp     string
		want    []string
		wantErr error
	}{
		{name: "relative to importing file with extension probing", imp: "./_defaults", want: []string{"orgs/acme/_defaults.yaml"}},
		{name: "relative parent", imp: "../acme/_defaults.yaml", want: []string{"orgs/acme/_defaults.yaml"}},
		{name: "base path relative with yml fallback", imp: "catalog/vpc", want: []string{"catalog/vpc.yml"}},
		{name: "extensionless template", imp: "catalog/tmpl-only", want: []string{"catalog/tmpl-only.yaml.tmpl"}},
		{name: "yml template", imp: "catalog/ytmpl", want: []string{"catalog/ytmpl.yml.tmpl"}},
		{name: "explicit yaml prefers tmpl sibling", imp: "catalog/both.yaml", want: []string{"catalog/both.yaml.tmpl"}},
		{name: "extensionless prefers plain yaml over template", imp: "catalog/both", want: []string{"catalog/both.yaml"}},
		{name: "glob adds default yaml extension", imp: "mixins/*", want: []string{"mixins/a.yaml"}},
		{name: "doublestar glob", imp: "mixins/**/*.yaml", want: []string{"mixins/a.yaml", "mixins/deep/b.yaml"}},
		{name: "missing", imp: "catalog/nope", wantErr: errUtils.ErrStackImportNotFound},
		{name: "glob no match", imp: "nothing/*", wantErr: errUtils.ErrStackImportNotFound},
		{name: "empty", imp: "", wantErr: errUtils.ErrInvalidImport},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveLocal(tt.imp, importer, root)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			want := make([]string, 0, len(tt.want))
			for _, w := range tt.want {
				want = append(want, filepath.Join(root, filepath.FromSlash(w)))
			}
			assert.ElementsMatch(t, want, got)
		})
	}
}

func TestResolveLocal_NoStacksBasePath(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "a/b.yaml", "a/c.yaml")
	importer := filepath.Join(root, "a", "b.yaml")

	_, err := ResolveLocal("a/c", importer, "")
	require.ErrorIs(t, err, errUtils.ErrInvalidImport)

	// Relative imports do not need the stacks base path.
	got, err := ResolveLocal("./c", importer, "")
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(root, "a", "c.yaml")}, got)
}

func TestIsTemplatedPathAndTemplateFile(t *testing.T) {
	assert.True(t, IsTemplatedPath("{{ .settings.x }}/vpc"))
	assert.False(t, IsTemplatedPath("catalog/vpc"))

	assert.True(t, IsTemplateFile("a/b.yaml.tmpl"))
	assert.True(t, IsTemplateFile("a/b.yml.tmpl"))
	assert.False(t, IsTemplateFile("a/b.yaml"))
}
