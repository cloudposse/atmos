package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// TestLoadConfig_ArtifactsMergeKeysNeverReplayDiscardedOrigins covers the precedence of origin
// records across merge keys: a key the mapping declares itself keeps its own origin, and a key
// copied from a merge source keeps the origin of the earliest source that supplies it.
func TestLoadConfig_ArtifactsMergeKeysNeverReplayDiscardedOrigins(t *testing.T) {
	const bases = `    base: &base
      kind: local/dir
      options:
        path: store
      verification:
        trusted_keys: !include ./defaults/keys.yaml
    other: &other
      kind: local/dir
      options:
        path: store
      verification:
        trusted_keys: !include ./other/keys.yaml
    empty: &empty
      kind: local/dir
      options:
        path: store
`
	tests := []struct {
		name string
		// repository is the body of the repository mapping in atmos.yaml.
		repository string
		// wantDir is the directory, relative to the config, that declares the effective keys.
		wantDir  string
		wantKeys string
	}{
		{
			name: "explicit keys declared before the merge key",
			repository: `      verification:
        trusted_keys: [release.pub]
      <<: *base
`,
			wantDir:  ".",
			wantKeys: "release.pub",
		},
		{
			name: "explicit keys declared after the merge key",
			repository: `      <<: *base
      verification:
        trusted_keys: [release.pub]
`,
			wantDir:  ".",
			wantKeys: "release.pub",
		},
		{
			name:       "keys supplied only by the merge source keep the source file",
			repository: "      <<: *base\n",
			wantDir:    "defaults",
			wantKeys:   "default.pub",
		},
		{
			name:       "the earlier of two merge sources supplies the origin",
			repository: "      <<: [*base, *other]\n",
			wantDir:    "defaults",
			wantKeys:   "default.pub",
		},
		{
			name:       "the later merge source supplies keys the earlier one lacks",
			repository: "      <<: [*empty, *other]\n",
			wantDir:    "other",
			wantKeys:   "other.pub",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := artifactsTestDir(t, "artifacts:\n  repositories:\n"+bases+"    r:\n"+tt.repository)
			writeArtifactsFile(t, filepath.Join(dir, "defaults", "keys.yaml"), "- default.pub\n")
			writeArtifactsFile(t, filepath.Join(dir, "other", "keys.yaml"), "- other.pub\n")

			cfg, err := loadArtifactsDir(t, dir)
			require.NoError(t, err)
			repo := cfg.Artifacts.Repositories["r"]
			require.NotNil(t, repo.Verification)
			wantDir := filepath.Join(dir, tt.wantDir)
			assert.Equal(t, []string{filepath.Join(wantDir, tt.wantKeys)}, repo.Verification.TrustedKeys)
			assert.Equal(t, wantDir, repo.Verification.KeysOrigin.Dir)
			assert.False(t, repo.Verification.KeysOrigin.Remote)
			assert.Equal(t, filepath.Join(dir, "store"), repo.Options["path"])
		})
	}
}

// TestDecodeNodeWithYamlFunctions_MergeKeyOriginRecords checks the origin records directly: only
// the keys a merge actually copies are reported, and an explicit key is reported exactly once.
func TestDecodeNodeWithYamlFunctions_MergeKeyOriginRecords(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{
			name:   "explicit key before the merge key",
			source: "child:\n  a: 1\n  <<: {a: 2, b: 3}\n",
			want:   []string{"child.a", "child.b"},
		},
		{
			name:   "explicit key after the merge key",
			source: "base: &base {a: 2, b: 3}\nchild:\n  <<: *base\n  a: 1\n",
			want:   []string{"child.a", "child.b"},
		},
		{
			name:   "nested mapping of a discarded key reports nothing",
			source: "base: &base {n: {x: 1}, m: 2}\nchild:\n  <<: *base\n  n: 5\n",
			want:   []string{"child.n", "child.m"},
		},
		{
			name:   "nested mapping of a copied key is reported",
			source: "base: &base {n: {x: 1}}\nchild:\n  <<: *base\n",
			want:   []string{"child.n", "child.n.x"},
		},
		{
			name:   "earlier merge sources win",
			source: "x: &x {a: {p: 1}}\ny: &y {a: {q: 1}, b: 1}\nchild:\n  <<: [*x, *y]\n",
			want:   []string{"child.a", "child.a.p", "child.b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen []string
			document := mustParseYAML(t, tt.source)
			decoder := configYamlDecoder{includes: map[string]bool{}}
			decoder.origin = func(path []string, _ string, _ bool) {
				if joined := joinPath(path); strings.HasPrefix(joined, "child.") {
					seen = append(seen, joined)
				}
			}
			_, err := decoder.decode(&document, filepath.Join(t.TempDir(), "atmos.yaml"))
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, seen, "each key's origin is reported exactly once, by the source that supplied it")
		})
	}
}

func mustParseYAML(t *testing.T, source string) yaml.Node {
	t.Helper()
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(source), &node))
	return node
}

func joinPath(path []string) string {
	out := ""
	for i, part := range path {
		if i > 0 {
			out += "."
		}
		out += part
	}
	return out
}
