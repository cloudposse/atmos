package engine

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func TestSensitiveInputsSelectExposedInclude(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "repo")
	unit := filepath.Join(root, "app", unitFile)
	parent := filepath.Join(root, "parent.hcl")
	for _, test := range []struct {
		name   string
		path   string
		expose string
		found  bool
	}{
		{"ancestor", `find_in_parent_folders("parent.hcl")`, "true", true},
		{"absolute", fmt.Sprintf("%q", filepath.ToSlash(parent)), "true", true},
		{"missing", `"missing.hcl"`, "true", false},
		{"hidden", `"parent.hcl"`, "false", false},
		{"unknown exposure", `"parent.hcl"`, "local.expose", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fsys := afero.NewMemMapFs()
			require.NoError(t, fsys.MkdirAll(filepath.Dir(unit), 0o755))
			require.NoError(t, afero.WriteFile(fsys, parent, []byte(`locals { token = sensitive("private") }`), 0o644))
			content := fmt.Sprintf("include \"root\" {\n path = %s\n expose = %s\n}\ninputs = { token = include.root.locals.token }", test.path, test.expose)
			require.NoError(t, afero.WriteFile(fsys, unit, []byte(content), 0o644))
			paths := SensitiveInputs(fsys, []string{unit, parent}, map[string]any{"token": "private"})
			if test.found {
				require.Equal(t, [][]string{{"token"}}, paths)
			} else {
				require.Empty(t, paths)
			}
		})
	}
}
