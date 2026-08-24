package sarif

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/hooks"
)

func TestCustomFormatOutputPath(t *testing.T) {
	outputDir := t.TempDir()
	outputFile := filepath.Join(outputDir, "default.sarif")

	t.Run("nil ctx or hook returns empty", func(t *testing.T) {
		assert.Empty(t, customFormatOutputPath(nil))
		assert.Empty(t, customFormatOutputPath(&hooks.ExecContext{}))
	})

	t.Run("uses output file when results is empty", func(t *testing.T) {
		got := customFormatOutputPath(&hooks.ExecContext{
			Hook:       &hooks.Hook{},
			OutputDir:  outputDir,
			OutputFile: outputFile,
		})

		assert.Equal(t, outputFile, got)
	})

	t.Run("joins valid relative results under output dir", func(t *testing.T) {
		got := customFormatOutputPath(&hooks.ExecContext{
			Hook:      &hooks.Hook{Results: "nested/results.sarif"},
			OutputDir: outputDir,
		})

		assert.Equal(t, filepath.Join(outputDir, "nested", "results.sarif"), got)
	})

	t.Run("rejects parent traversal results", func(t *testing.T) {
		for _, results := range []string{"../outside.sarif", "nested/../../outside.sarif"} {
			t.Run(results, func(t *testing.T) {
				got := customFormatOutputPath(&hooks.ExecContext{
					Hook:      &hooks.Hook{Results: results},
					OutputDir: outputDir,
				})

				assert.Empty(t, got)
			})
		}
	})

	t.Run("rejects absolute results", func(t *testing.T) {
		absolute, err := filepath.Abs("outside.sarif")
		require.NoError(t, err)

		got := customFormatOutputPath(&hooks.ExecContext{
			Hook:      &hooks.Hook{Results: absolute},
			OutputDir: outputDir,
		})

		assert.Empty(t, got)
	})
}
