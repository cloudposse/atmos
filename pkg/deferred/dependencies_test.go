package deferred

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Domain implementations depend on deferred, never the reverse. Auth must not
// parse declarations or bind store clients on behalf of those domains.
func TestSubsystemDependencyDirection(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	require.True(t, ok)
	pkg := filepath.Dir(filepath.Dir(source))
	for _, rule := range []struct {
		path      string
		forbidden []string
	}{
		{filepath.Join(pkg, "deferred"), []string{"auth", "store", "secrets", "schema"}},
		{filepath.Join(pkg, "auth", "deferred"), []string{"store", "secrets"}},
		{filepath.Join(pkg, "store", "deferred"), []string{"secrets"}},
	} {
		entries, err := os.ReadDir(rule.path)
		require.NoError(t, err)
		checked := 0
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(rule.path, entry.Name()), nil, parser.ImportsOnly)
			require.NoError(t, err)
			checked++
			for _, imp := range file.Imports {
				path, err := strconv.Unquote(imp.Path.Value)
				require.NoError(t, err)
				for _, domain := range rule.forbidden {
					prefix := "github.com/cloudposse/atmos/pkg/" + domain
					require.False(t, path == prefix || strings.HasPrefix(path, prefix+"/"), "%s imports forbidden dependency %s", filepath.Join(rule.path, entry.Name()), path)
				}
			}
		}
		require.Positive(t, checked)
	}
}
