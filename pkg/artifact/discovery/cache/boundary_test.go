package cache_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCacheStaysNeutral keeps the shared snapshot mechanics free of any plugin or artifact schema,
// so both plugin discovery and artifact discovery can build on it without a dependency cycle.
func TestCacheStaysNeutral(t *testing.T) {
	forbidden := []string{
		"github.com/cloudposse/atmos/cmd",
		"github.com/cloudposse/atmos/pkg/plugin",
		"github.com/cloudposse/atmos/pkg/artifact/service",
		"github.com/cloudposse/atmos/pkg/artifact/discovery/catalog",
		"github.com/cloudposse/atmos/pkg/schema",
		"github.com/cloudposse/atmos/pkg/auth",
		"github.com/google/go-containerregistry",
		"oras.land/oras-go",
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
				require.Falsef(t, path == prefix || strings.HasPrefix(path, prefix+"/"), "%s imports %s", file, path)
			}
		}
	}
	require.Positive(t, checked, "no source files were checked")
}
