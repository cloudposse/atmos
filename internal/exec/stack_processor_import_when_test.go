package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProcessImportSection_When(t *testing.T) {
	section := func(imports ...any) map[string]any {
		return map[string]any{"import": imports}
	}
	paths := func(t *testing.T, stackMap map[string]any) []string {
		t.Helper()
		imports, err := ProcessImportSection(stackMap, "stacks/dev.yaml")
		require.NoError(t, err)
		out := make([]string, 0, len(imports))
		for _, imp := range imports {
			out = append(out, imp.Path)
		}
		return out
	}

	t.Run("keeps strings, unconditional objects, and matching conditions in order", func(t *testing.T) {
		t.Cleanup(ci.SwapRegistryForTest())

		got := paths(t, section(
			"catalog/base",
			map[string]any{"path": "mixins/plain"},
			map[string]any{"path": "mixins/always", "when": "always"},
			map[string]any{"path": "mixins/local", "when": "local"},
		))
		assert.Equal(t, []string{"catalog/base", "mixins/plain", "mixins/always", "mixins/local"}, got)
	})

	t.Run("drops imports whose condition does not match", func(t *testing.T) {
		t.Cleanup(ci.SwapRegistryForTest())

		got := paths(t, section(
			"catalog/base",
			map[string]any{"path": "mixins/never", "when": "never"},
			map[string]any{"path": "mixins/ci", "when": "ci"},
			"catalog/last",
		))
		assert.Equal(t, []string{"catalog/base", "catalog/last"}, got)
	})

	t.Run("a stack with no conditional imports does not need runtime facts", func(t *testing.T) {
		got := paths(t, section("catalog/a", map[string]any{"path": "catalog/b"}))
		assert.Equal(t, []string{"catalog/a", "catalog/b"}, got)
	})

	t.Run("an invalid condition is reported with the file and the import", func(t *testing.T) {
		_, err := ProcessImportSection(section(map[string]any{"path": "mixins/x", "when": "sometimes"}), "stacks/dev.yaml")
		require.ErrorIs(t, err, errUtils.ErrInvalidImport)
		assert.Contains(t, err.Error(), "stacks/dev.yaml")
		assert.Contains(t, err.Error(), "mixins/x")
	})

	t.Run("an expression over an unset variable skips the import", func(t *testing.T) {
		t.Cleanup(ci.SwapRegistryForTest())

		got := paths(t, section(
			"catalog/base",
			map[string]any{"path": "mixins/x", "when": "env.ATMOS_TEST_UNSET_VARIABLE == 'x'"},
		))
		assert.Equal(t, []string{"catalog/base"}, got)
	})
}

func TestProcessImportSection_WhenPreservesSourceIndex(t *testing.T) {
	imports, err := ProcessImportSection(map[string]any{"import": []any{
		map[string]any{"path": "missing", "when": "never"},
		"first",
		map[string]any{"path": "also-missing", "when": "never"},
		map[string]any{"path": "second", "when": "always", "SourceIndex": 99},
	}}, "stack.yaml")
	require.NoError(t, err)
	require.Len(t, imports, 2)
	assert.Equal(t, 1, imports[0].SourceIndex)
	assert.Equal(t, 3, imports[1].SourceIndex)
}

func TestConditionalImportProvenance(t *testing.T) {
	dir := t.TempDir()
	manifest := "import:\n  - path: missing\n    when: !cel labels.pci == true\n  - first\n  - path: also-missing\n    when: never\n  - path: second\n    when: !cel labels.pci == false\nmetadata: {labels: {pci: false}}\n"
	path := filepath.Join(dir, "manifest.yaml")
	require.NoError(t, os.WriteFile(path, []byte(manifest), 0o600))
	for _, name := range []string{"first", "second"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".yaml"), []byte("vars: {}\n"), 0o600))
	}
	config := &schema.AtmosConfiguration{TrackProvenance: true}
	_, ctx, err := ProcessYAMLConfigFileWithContext(
		config, dir, path, map[string]map[string]any{}, nil,
		false, false, false, false, nil, nil, nil, nil, "", nil,
	)
	require.NoError(t, err)
	require.NotNil(t, ctx)
	for name, line := range map[string]int{"first": 4, "second": 7} {
		entries := ctx.GetProvenance("__import__:" + name)
		require.NotEmpty(t, entries, name)
		assert.Equal(t, line, entries[0].Line, name)
		assert.Equal(t, 5, entries[0].Column, name)
	}
	assert.False(t, ctx.HasProvenance("__import__:missing"))
	assert.False(t, ctx.HasProvenance("__import__:also-missing"))
}
