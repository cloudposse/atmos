package service_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestArtifactPackagesImportNoCLIRunnerHookPluginOrCI keeps the artifact service and the packages
// it composes (preparation, staging, materialization) free of CLI, runner, hook, plugin, and CI
// dependencies, as the package boundary contract requires. Test-support packages are checked too.
func TestArtifactPackagesImportNoCLIRunnerHookPluginOrCI(t *testing.T) {
	forbidden := []string{
		"github.com/cloudposse/atmos/cmd",
		"github.com/cloudposse/atmos/pkg/ci",
		"github.com/cloudposse/atmos/pkg/plugin",
		"github.com/cloudposse/atmos/pkg/runner",
		"github.com/cloudposse/atmos/pkg/hooks",
		"github.com/cloudposse/atmos/internal/exec",
	}
	files, err := filepath.Glob(filepath.Join("..", "*", "*.go"))
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
