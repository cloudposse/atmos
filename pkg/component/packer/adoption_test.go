package packer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackerTemplateShapes(t *testing.T) {
	for _, tc := range []struct{ name, code, want string }{
		{"split.pkr.hcl", "variable \"x\" {}", "0 build sources"},
		{"broken.pkr.hcl", "build {", "invalid component"},
		{"dynamic.pkr.hcl", "build { sources = var.sources }", "literal list"},
		{"many.pkr.hcl", "build { sources = [\"source.file.a\", \"source.file.b\"] }", "2 build sources"},
		{"one.pkr.hcl", "build {\n source \"source.file.a\" {}\n}", ""},
		{"json.pkr.json", `{"build":[{"sources":["source.file.a"]}]}`, ""},
		{"json.pkr.hcl", "build \"x\" {}", "invalid component"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			file := filepath.Join(dir, tc.name)
			write(t, file, tc.code)
			entry, err := packerEntry(file, dir, dir)
			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)
				return
			}
			require.NoError(t, err)
			require.Equal(t, map[string]any{"packer": map[string]any{"template": tc.name}}, entry["settings"])
		})
	}
}

func write(t *testing.T, path, text string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
}
