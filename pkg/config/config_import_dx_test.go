package config

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestProcessImports_RequiredVsOptional verifies the fail-open (string/optional) vs fail-closed
// (object/required) semantics of CLI-config imports (issue #3430, proposal #1).
func TestProcessImports_RequiredVsOptional(t *testing.T) {
	setupTestAdapters()
	t.Cleanup(ResetImportAdapterRegistry)

	tests := []struct {
		name      string
		imp       schema.ConfigImport
		wantError bool
	}{
		{
			name:      "string-form missing import is optional (warn and skip)",
			imp:       schema.ConfigImport{Path: "missing.yaml", SkipIfMissing: true},
			wantError: false,
		},
		{
			name:      "object-form missing import is required (fail closed)",
			imp:       schema.ConfigImport{Path: "missing.yaml", SkipIfMissing: false},
			wantError: true,
		},
		{
			name:      "object-form missing import with skip_if_missing is optional",
			imp:       schema.ConfigImport{Path: "missing.yaml", SkipIfMissing: true},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := t.TempDir()
			paths, err := processImports(nil, baseDir, []schema.ConfigImport{tt.imp}, t.TempDir(), 1, MaximumImportLvL)
			if tt.wantError {
				require.ErrorIs(t, err, errUtils.ErrRequiredImportFailed)
				return
			}
			require.NoError(t, err)
			assert.Empty(t, paths, "a missing optional import resolves to nothing")
		})
	}
}

// TestProcessImports_WhenGating verifies that a per-import `when` condition gates whether the
// import is resolved at all (issue #3430, proposal #4).
func TestProcessImports_WhenGating(t *testing.T) {
	setupTestAdapters()
	t.Cleanup(ResetImportAdapterRegistry)

	t.Run("when: never skips a required missing import without error", func(t *testing.T) {
		baseDir := t.TempDir()
		imp := schema.ConfigImport{Path: "missing.yaml", When: schema.MustCondition("never")}
		paths, err := processImports(nil, baseDir, []schema.ConfigImport{imp}, t.TempDir(), 1, MaximumImportLvL)
		require.NoError(t, err, "a skipped import is never resolved, so its missing target is not an error")
		assert.Empty(t, paths)
	})

	t.Run("when: always resolves a required missing import and fails closed", func(t *testing.T) {
		baseDir := t.TempDir()
		imp := schema.ConfigImport{Path: "missing.yaml", When: schema.MustCondition("always")}
		_, err := processImports(nil, baseDir, []schema.ConfigImport{imp}, t.TempDir(), 1, MaximumImportLvL)
		require.ErrorIs(t, err, errUtils.ErrRequiredImportFailed)
	})

	t.Run("when: local matches off-CI and resolves the import", func(t *testing.T) {
		// Clear the CI provider registry so ci.IsCI() is deterministically false.
		restore := ci.SwapRegistryForTest()
		t.Cleanup(restore)

		baseDir := t.TempDir()
		imp := schema.ConfigImport{Path: "missing.yaml", When: schema.MustCondition("local")}
		_, err := processImports(nil, baseDir, []schema.ConfigImport{imp}, t.TempDir(), 1, MaximumImportLvL)
		require.ErrorIs(t, err, errUtils.ErrRequiredImportFailed, "local matches off-CI, so the required import is resolved and fails closed")
	})

	t.Run("when: !cel matching env var resolves the import", func(t *testing.T) {
		t.Setenv("ATMOS_TEST_IMPORT_ENV", "prod")

		baseDir := t.TempDir()
		imp := schema.ConfigImport{Path: "missing.yaml", When: schema.MustCondition(`!cel env.ATMOS_TEST_IMPORT_ENV == "prod"`)}
		_, err := processImports(nil, baseDir, []schema.ConfigImport{imp}, t.TempDir(), 1, MaximumImportLvL)
		require.ErrorIs(t, err, errUtils.ErrRequiredImportFailed, "the CEL env expression matches, so the required import is resolved and fails closed")
	})

	t.Run("when: !cel non-matching env var skips the import", func(t *testing.T) {
		t.Setenv("ATMOS_TEST_IMPORT_ENV", "dev")

		baseDir := t.TempDir()
		imp := schema.ConfigImport{Path: "missing.yaml", When: schema.MustCondition(`!cel env.ATMOS_TEST_IMPORT_ENV == "prod"`)}
		paths, err := processImports(nil, baseDir, []schema.ConfigImport{imp}, t.TempDir(), 1, MaximumImportLvL)
		require.NoError(t, err, "the CEL env expression does not match, so the import is skipped and never resolved")
		assert.Empty(t, paths)
	})
}

// TestNormalizeConfigImports verifies both the legacy string-list form and the new object form
// decode correctly through the raw-value path used by adapters and profile loading.
func TestNormalizeConfigImports(t *testing.T) {
	t.Run("nil returns nil", func(t *testing.T) {
		out, err := NormalizeConfigImports(nil)
		require.NoError(t, err)
		assert.Nil(t, out)
	})

	t.Run("string slice becomes optional imports", func(t *testing.T) {
		out, err := NormalizeConfigImports([]string{"a.yaml", "b.yaml"})
		require.NoError(t, err)
		require.Len(t, out, 2)
		assert.Equal(t, "a.yaml", out[0].Path)
		assert.True(t, out[0].SkipIfMissing)
		assert.Equal(t, "b.yaml", out[1].Path)
		assert.True(t, out[1].SkipIfMissing)
	})

	t.Run("mixed string and object entries", func(t *testing.T) {
		out, err := NormalizeConfigImports([]any{
			"legacy.yaml",
			map[string]any{"path": "required.yaml"},
			map[string]any{"path": "opt.yaml", "skip_if_missing": true, "when": "local"},
		})
		require.NoError(t, err)
		require.Len(t, out, 3)
		assert.Equal(t, "legacy.yaml", out[0].Path)
		assert.True(t, out[0].SkipIfMissing)
		assert.Equal(t, "required.yaml", out[1].Path)
		assert.False(t, out[1].SkipIfMissing, "object import defaults to fail-closed")
		assert.Equal(t, "opt.yaml", out[2].Path)
		assert.True(t, out[2].SkipIfMissing)
		assert.False(t, out[2].When.IsZero())
	})

	t.Run("empty string entries are dropped", func(t *testing.T) {
		out, err := NormalizeConfigImports([]any{"", "kept.yaml"})
		require.NoError(t, err)
		require.Len(t, out, 1)
		assert.Equal(t, "kept.yaml", out[0].Path)
	})

	t.Run("malformed object entry is an error, not a silent drop", func(t *testing.T) {
		// An uncompilable CEL `when` makes the entry fail to decode; it must surface as an error
		// so a required entry is not silently converted into an optional omission.
		_, err := NormalizeConfigImports([]any{
			map[string]any{"path": "bad.yaml", "when": "!cel ((("},
		})
		require.ErrorIs(t, err, errUtils.ErrInvalidConfigImport)
	})
}

// TestProcessImports_OptionalParentDoesNotMaskFailClosedNested verifies that a fail-closed nested
// error surfaced by an adapter processing nested imports propagates even when the importing
// (parent) entry is optional (issue #3430 review follow-up). Both a required nested failure
// (ErrRequiredImportFailed) and a malformed nested entry (ErrInvalidConfigImport) must not be
// masked by an optional parent.
func TestProcessImports_OptionalParentDoesNotMaskFailClosedNested(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"required nested failure", fmt.Errorf("%w: nested required import", errUtils.ErrRequiredImportFailed), errUtils.ErrRequiredImportFailed},
		{"malformed nested entry", fmt.Errorf("%w: bad when", errUtils.ErrInvalidConfigImport), errUtils.ErrInvalidConfigImport},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ResetImportAdapterRegistry()
			t.Cleanup(ResetImportAdapterRegistry)
			RegisterImportAdapter(&recordingSchemeAdapter{schemes: []string{"https://"}, err: tc.err})

			tempDir := t.TempDir()
			// The parent entry is optional, but the adapter surfaces a fail-closed nested error.
			imp := schema.ConfigImport{Path: "https://example.com/parent.yaml", SkipIfMissing: true}
			_, err := processImports(nil, tempDir, []schema.ConfigImport{imp}, tempDir, 1, MaximumImportLvL)
			require.ErrorIs(t, err, tc.want, "an optional parent must not mask a fail-closed nested error")
		})
	}
}

// TestProcessImports_StringImportStillResolvesExistingFile is a backward-compatibility guard:
// a legacy string import that resolves to a real file still merges as before.
func TestProcessImports_StringImportStillResolvesExistingFile(t *testing.T) {
	setupTestAdapters()
	t.Cleanup(ResetImportAdapterRegistry)

	baseDir := t.TempDir()
	_, err := setupTestFile("key: value\n", baseDir, "present.yaml")
	require.NoError(t, err)

	paths, err := processImports(nil, baseDir, strImports("present.yaml"), t.TempDir(), 1, MaximumImportLvL)
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.Equal(t, filepath.Join(baseDir, "present.yaml"), paths[0].FilePath)
	assert.True(t, paths[0].SkipIfMissing, "string imports stay optional")
}
