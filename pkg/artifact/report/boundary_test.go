package report

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

// TestOnlyReportDependsOnCIReporting enforces the package boundary: pkg/artifact/report is the only
// package under pkg/artifact that imports CI reporting, so the domain, services, and wiring stay
// free of it. A new package that needs to report must record into a [Collector] instead.
func TestOnlyReportDependsOnCIReporting(t *testing.T) {
	root, err := filepath.Abs("..")
	require.NoError(t, err)
	checked, withCI := 0, 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		require.NoError(t, err)
		inReport := strings.HasPrefix(filepath.ToSlash(relative), "report/")
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		require.NoError(t, err)
		checked++
		for _, spec := range parsed.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			require.NoError(t, err)
			if imported != "github.com/cloudposse/atmos/pkg/ci" && !strings.HasPrefix(imported, "github.com/cloudposse/atmos/pkg/ci/") {
				continue
			}
			if inReport {
				withCI++
				continue
			}
			require.Failf(t, "forbidden import", "%s imports %s; only pkg/artifact/report may depend on CI reporting", filepath.ToSlash(relative), imported)
		}
		return nil
	})
	require.NoError(t, err)
	require.Positive(t, checked, "no source files were checked")
	require.Positive(t, withCI, "the report package no longer imports CI reporting; update this test")
}
