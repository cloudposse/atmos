package artifact

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPackageDependencyBoundary keeps the domain package free of CLI, runner, hook, plugin, and CI
// dependencies, as required by the package boundary contract.
func TestPackageDependencyBoundary(t *testing.T) {
	forbidden := []string{
		"github.com/cloudposse/atmos/cmd",
		"github.com/cloudposse/atmos/pkg/ci",
		"github.com/cloudposse/atmos/pkg/plugin",
		"github.com/cloudposse/atmos/pkg/runner",
		"github.com/cloudposse/atmos/pkg/hooks",
		"github.com/cloudposse/atmos/internal/exec",
		"github.com/google/go-containerregistry",
		"oras.land/oras-go",
		"github.com/aws/aws-sdk-go-v2",
	}
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	checked := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		require.NoError(t, err)
		checked++
		for _, spec := range parsed.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			require.NoError(t, err)
			for _, prefix := range forbidden {
				require.Falsef(t, path == prefix || strings.HasPrefix(path, prefix+"/"), "%s imports forbidden package %s", file, path)
			}
		}
	}
	require.Positive(t, checked, "no source files were checked")
}
