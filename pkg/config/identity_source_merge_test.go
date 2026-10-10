package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergeAuthIdentitiesFromSource_SkipsUnusableEntries verifies that one
// unusable entry never discards the identities that can still be read.
func TestMergeAuthIdentitiesFromSource_SkipsUnusableEntries(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    map[string]string
	}{
		{
			name:    "source that is not valid YAML",
			content: "auth: [unterminated",
			want:    map[string]string{},
		},
		{
			name:    "source without identities",
			content: "auth:\n  providers: {}\n",
			want:    map[string]string{},
		},
		{
			name:    "identities that are not a mapping",
			content: "auth:\n  identities: [reader]\n",
			want:    map[string]string{},
		},
		{
			name: "identity that is not a mapping",
			content: `auth:
  identities:
    broken: just-a-string
    reader:
      kind: mock
`,
			want: map[string]string{"reader": "mock"},
		},
		{
			name: "identity with an unsupported YAML function",
			content: `auth:
  identities:
    broken:
      kind: !not-an-atmos-function value
    reader:
      kind: mock
`,
			want: map[string]string{"reader": "mock"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identities := map[string]map[string]any{}

			mergeAuthIdentitiesFromSource(mergedConfigSource{path: "test.yaml", content: tt.content}, identities)

			got := map[string]string{}
			for name, identity := range identities {
				kind, _ := identity["kind"].(string)
				got[name] = kind
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestMergeAuthIdentitiesFromSource_LowercasesNamesAndMergesInOrder pins the
// merge contract: names match case-insensitively and later sources win field by
// field while keeping fields they do not mention.
func TestMergeAuthIdentitiesFromSource_LowercasesNamesAndMergesInOrder(t *testing.T) {
	identities := map[string]map[string]any{}

	mergeAuthIdentitiesFromSource(mergedConfigSource{path: "base.yaml", content: "auth:\n  identities:\n    Reader:\n      kind: mock\n      default: true\n      principal:\n        name: from-base\n"}, identities)
	mergeAuthIdentitiesFromSource(mergedConfigSource{path: "override.yaml", content: "auth:\n  identities:\n    READER:\n      default: false\n      principal:\n        name: from-override\n"}, identities)

	require.Contains(t, identities, "reader")
	require.Len(t, identities, 1)
	reader := identities["reader"]
	assert.Equal(t, "mock", reader["kind"], "fields the later source omits are kept")
	assert.Equal(t, false, reader["default"], "an explicit false overrides an earlier true")
	assert.Equal(t, map[string]any{"name": "from-override"}, reader["principal"])
}

// TestCollectConfigSourcesForCasePreservation_FallsBackToMainConfig covers
// callers that have no tracker for their Viper instance.
func TestCollectConfigSourcesForCasePreservation_FallsBackToMainConfig(t *testing.T) {
	dir := t.TempDir()
	mainConfig := filepath.Join(dir, "atmos.yaml")
	require.NoError(t, os.WriteFile(mainConfig, []byte("base_path: ./\n"), 0o600))

	t.Run("reads the main config when nothing was tracked", func(t *testing.T) {
		sources := collectConfigSourcesForCasePreservation(viper.New(), mainConfig)

		require.Len(t, sources, 1)
		assert.Equal(t, mainConfig, sources[0].path)
		assert.Equal(t, "base_path: ./\n", sources[0].content)
	})

	t.Run("returns nothing when the main config is unreadable", func(t *testing.T) {
		sources := collectConfigSourcesForCasePreservation(viper.New(), filepath.Join(dir, "missing.yaml"))

		assert.Empty(t, sources)
	})

	t.Run("returns nothing without a main config", func(t *testing.T) {
		assert.Empty(t, collectConfigSourcesForCasePreservation(viper.New(), ""))
	})

	t.Run("does not duplicate a tracked main config", func(t *testing.T) {
		v := viper.New()
		resetMergedConfigFiles(v)
		mergedFilesReg.track(v, mainConfig, []byte("tracked: true\n"))
		t.Cleanup(func() { mergedFilesReg.finish(v) })

		sources := collectConfigSourcesForCasePreservation(v, mainConfig)

		require.Len(t, sources, 1)
		assert.Equal(t, "tracked: true\n", sources[0].content, "the retained content wins over the file on disk")
	})
}
