package blob

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTransportHasNoCIOrCommandDependencies keeps the dependency direction one way: CI adapters
// import the blob transport, never the reverse. The blob packages (including every backend and the
// S3 fake) must not reach for CI, command, hook, plugin, runner, or exec packages.
func TestTransportHasNoCIOrCommandDependencies(t *testing.T) {
	forbidden := []string{
		"github.com/cloudposse/atmos/cmd",
		"github.com/cloudposse/atmos/pkg/ci",
		"github.com/cloudposse/atmos/pkg/plugin",
		"github.com/cloudposse/atmos/pkg/runner",
		"github.com/cloudposse/atmos/pkg/hooks",
		"github.com/cloudposse/atmos/internal/exec",
	}
	checked := 0
	require.NoError(t, filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		require.NoError(t, parseErr)
		checked++
		for _, spec := range parsed.Imports {
			imported, unquoteErr := strconv.Unquote(spec.Path.Value)
			require.NoError(t, unquoteErr)
			for _, prefix := range forbidden {
				require.Falsef(t, imported == prefix || strings.HasPrefix(imported, prefix+"/"), "%s imports forbidden package %s", path, imported)
			}
		}
		return nil
	}))
	require.Greater(t, checked, 20, "the boundary check must walk the backends and test helpers")
}
