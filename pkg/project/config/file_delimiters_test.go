package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Compile-time sentinel so renaming FileSpec.Delimiters fails the build.
var _ = FileSpec{Delimiters: []string{"[[", "]]"}}

func TestFileSpecResolveDelimiters(t *testing.T) {
	active := []string{"{{", "}}"}

	tests := []struct {
		name     string
		spec     FileSpec
		fallback []string
		want     []string
	}{
		{name: "file pair wins over the fallback", spec: FileSpec{Delimiters: []string{"[[", "]]"}}, fallback: active, want: []string{"[[", "]]"}},
		{name: "unset file pair falls back", spec: FileSpec{}, fallback: active, want: active},
		{name: "one-element file pair falls back", spec: FileSpec{Delimiters: []string{"[["}}, fallback: active, want: active},
		{name: "three-element file pair falls back", spec: FileSpec{Delimiters: []string{"[[", "]]", "x"}}, fallback: active, want: active},
		{name: "empty left delimiter falls back", spec: FileSpec{Delimiters: []string{"", "]]"}}, fallback: active, want: active},
		{name: "empty right delimiter falls back", spec: FileSpec{Delimiters: []string{"[[", ""}}, fallback: active, want: active},
		{name: "custom fallback is returned untouched", spec: FileSpec{}, fallback: []string{"<<", ">>"}, want: []string{"<<", ">>"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.spec.ResolveDelimiters(tt.fallback))
		})
	}
}

// TestFileSpecResolveDelimiters_ReturnsCopy proves mutating the result never
// changes the manifest's own slice.
func TestFileSpecResolveDelimiters_ReturnsCopy(t *testing.T) {
	spec := FileSpec{Delimiters: []string{"[[", "]]"}}

	got := spec.ResolveDelimiters([]string{"{{", "}}"})
	got[0] = "mutated"

	assert.Equal(t, []string{"[[", "]]"}, spec.Delimiters)
}

func TestLoadScaffoldConfig_PerFileDelimiters(t *testing.T) {
	const header = "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n"

	t.Run("accepts a two-element per-file pair", func(t *testing.T) {
		content := header + "  delimiters: [\"{{\", \"}}\"]\n  files:\n" +
			"    - path: \"charts/**\"\n      delimiters: [\"[[\", \"]]\"]\n" +
			"    - path: \".github/workflows/*.yml.tmpl\"\n      delimiters: [\"<<\", \">>\"]\n"

		scaffoldConfig, err := LoadScaffoldConfigFromContent(content)

		require.NoError(t, err)
		require.Len(t, scaffoldConfig.Spec.Files, 2)
		assert.Equal(t, []string{"[[", "]]"}, scaffoldConfig.Spec.Files[0].Delimiters)
		assert.Equal(t, []string{"<<", ">>"}, scaffoldConfig.Spec.Files[1].Delimiters)
	})

	rejected := []struct {
		name       string
		delimiters string
		wantErr    error
	}{
		{name: "rejects a one-element pair", delimiters: `["[["]`, wantErr: errUtils.ErrManifestValidation},
		{name: "rejects a three-element pair", delimiters: `["[[", "]]", "x"]`, wantErr: errUtils.ErrManifestValidation},
		{name: "rejects an empty list", delimiters: `[]`, wantErr: errUtils.ErrManifestValidation},
		{name: "rejects an empty left delimiter", delimiters: `["", "]]"]`, wantErr: errUtils.ErrScaffoldFileDelimitersInvalid},
		{name: "rejects an empty right delimiter", delimiters: `["[[", ""]`, wantErr: errUtils.ErrScaffoldFileDelimitersInvalid},
	}
	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			content := header + "  files:\n    - path: \"charts/**\"\n      delimiters: " + tt.delimiters + "\n"

			_, err := LoadScaffoldConfigFromContent(content)

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestLoadScaffoldConfig_MatrixAxisUsesPerFileDelimiters proves a matrix axis
// expression written in a file's own delimiters is recognized as an expression
// even when spec.delimiters is the default, and that the same text is still
// rejected on an entry that declares no per-file pair (negative path).
func TestLoadScaffoldConfig_MatrixAxisUsesPerFileDelimiters(t *testing.T) {
	const header = "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n  files:\n"
	const entry = "    - path: deploy.yaml\n      target: \"deploy/[[ .matrix.environment ]].yaml\"\n" +
		"      matrix:\n        environment: '[[ collectKeys answers.environments ]]'\n"

	t.Run("accepted with a per-file pair", func(t *testing.T) {
		scaffoldConfig, err := LoadScaffoldConfigFromContent(header + entry + "      delimiters: [\"[[\", \"]]\"]\n")

		require.NoError(t, err)
		require.Len(t, scaffoldConfig.Spec.Files, 1)
		assert.Equal(t, "[[ collectKeys answers.environments ]]", scaffoldConfig.Spec.Files[0].Matrix["environment"])
	})

	t.Run("rejected without a per-file pair", func(t *testing.T) {
		_, err := LoadScaffoldConfigFromContent(header + entry)

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrScaffoldMatrixAxisInvalid)
	})

	t.Run("spec-level pair does not leak past a per-file override", func(t *testing.T) {
		content := "apiVersion: atmos/v1\nkind: AtmosScaffoldConfig\nmetadata:\n  name: test\nspec:\n" +
			"  delimiters: [\"[[\", \"]]\"]\n  files:\n" +
			"    - path: deploy.yaml\n      target: \"deploy/<< .matrix.environment >>.yaml\"\n" +
			"      delimiters: [\"<<\", \">>\"]\n" +
			"      matrix:\n        environment: '[[ collectKeys answers.environments ]]'\n"

		_, err := LoadScaffoldConfigFromContent(content)

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrScaffoldMatrixAxisInvalid)
	})
}
