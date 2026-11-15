package config

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	artifactconfig "github.com/cloudposse/atmos/pkg/artifact/config"
)

func TestLoadConfig_ArtifactsTaggedAliasIsEvaluated(t *testing.T) {
	store := filepath.Join(t.TempDir(), "store")
	t.Setenv("ARTIFACT_STORE", store)
	dir := artifactsTestDir(t, `artifacts:
  repositories:
    first:
      kind: local/dir
      options:
        path: &store !env ARTIFACT_STORE
    second:
      kind: local/dir
      options:
        path: *store
`)
	cfg, err := loadArtifactsDir(t, dir)
	require.NoError(t, err)
	assert.Equal(t, store, cfg.Artifacts.Repositories["first"].Options["path"])
	assert.Equal(t, store, cfg.Artifacts.Repositories["second"].Options["path"], "the alias sees the evaluated tag, not the literal variable name")
	require.NoError(t, artifactconfig.Validate(&cfg.Artifacts))
}

func TestLoadConfig_ArtifactsAnchoredExecEvaluatedOnceAcrossAliases(t *testing.T) {
	dir := t.TempDir()
	countPath := filepath.Join(dir, "count")
	t.Setenv(mergeConfigFileExecHelperEnv, "1")
	root := artifactsTestDir(t, fmt.Sprintf(`artifacts:
  repositories:
    first:
      kind: local/dir
      options:
        path: &store !exec >-
          %s -test.run=^TestMergeConfigFileExecHelper$ -- %s
    second:
      kind: local/dir
      options:
        path: *store
`, shellQuote(os.Args[0]), shellQuote(countPath)))
	cfg, err := loadArtifactsDir(t, root)
	require.NoError(t, err)
	count, err := os.ReadFile(countPath)
	require.NoError(t, err)
	assert.Equal(t, "1", string(count), "an alias reuses the anchor's result instead of running the command again")
	want := filepath.Join(root, "resolved-description")
	assert.Equal(t, want, cfg.Artifacts.Repositories["first"].Options["path"])
	assert.Equal(t, want, cfg.Artifacts.Repositories["second"].Options["path"])
}

func TestLoadConfig_ArtifactsAliasesAndMergeKeysKeepDeclaringFile(t *testing.T) {
	dir := artifactsTestDir(t, "import:\n  - ./trust/artifacts.yaml\n")
	writeArtifactsFile(t, filepath.Join(dir, "trust", "artifacts.yaml"), `artifacts:
  repositories:
    base: &base
      kind: local/dir
      options:
        path: store
      verification:
        trusted_keys: &keys [release.pub]
    aliased: *base
    inherited:
      <<: *base
    overridden:
      <<: *base
      options:
        path: other
    keyed:
      kind: local/dir
      options:
        path: keyed
      verification:
        trusted_keys: *keys
`)
	cfg, err := loadArtifactsDir(t, dir)
	require.NoError(t, err)

	trust := filepath.Join(dir, "trust")
	repos := cfg.Artifacts.Repositories
	for _, alias := range []string{"base", "aliased", "inherited"} {
		assert.Equal(t, "local/dir", repos[alias].Kind, alias)
		assert.Equal(t, filepath.Join(trust, "store"), repos[alias].Options["path"], alias)
		require.NotNil(t, repos[alias].Verification, alias)
		assert.Equal(t, []string{filepath.Join(trust, "release.pub")}, repos[alias].Verification.TrustedKeys, alias)
	}
	assert.Equal(t, "local/dir", repos["overridden"].Kind, "kind is inherited through the merge key")
	assert.Equal(t, filepath.Join(trust, "other"), repos["overridden"].Options["path"], "the repository's own options win over the merged ones")
	assert.Equal(t, []string{filepath.Join(trust, "release.pub")}, repos["keyed"].Verification.TrustedKeys)
	require.NoError(t, artifactconfig.Validate(&cfg.Artifacts))
}

func TestLoadConfig_ArtifactsTrustedKeysIncludeAnchorsToIncludedFile(t *testing.T) {
	for _, suffix := range []string{"", " | eval"} {
		t.Run(fmt.Sprintf("eval=%t", suffix != ""), func(t *testing.T) {
			dir := artifactsTestDir(t, fmt.Sprintf(`artifacts:
  verification:
    trusted_keys: !include ./trust/keys.yaml%s
  repositories:
    local:
      kind: local/dir
      options:
        path: store
      verification:
        trusted_keys: !include ./trust/repo-keys.yaml%s
`, suffix, suffix))
			writeArtifactsFile(t, filepath.Join(dir, "trust", "keys.yaml"), "- release.pub\n")
			writeArtifactsFile(t, filepath.Join(dir, "trust", "repo-keys.yaml"), "- repo.pub\n")
			// A same-named file beside the including config must not be the one trusted.
			writeArtifactsFile(t, filepath.Join(dir, "release.pub"), "wrong\n")

			cfg, err := loadArtifactsDir(t, dir)
			require.NoError(t, err)
			require.NotNil(t, cfg.Artifacts.Verification)
			assert.Equal(t, []string{filepath.Join(dir, "trust", "release.pub")}, cfg.Artifacts.Verification.TrustedKeys)
			assert.Equal(t, filepath.Join(dir, "trust"), cfg.Artifacts.Verification.KeysOrigin.Dir)
			assert.False(t, cfg.Artifacts.Verification.KeysOrigin.Remote)

			repo := cfg.Artifacts.Repositories["local"]
			require.NotNil(t, repo.Verification)
			assert.Equal(t, []string{filepath.Join(dir, "trust", "repo.pub")}, repo.Verification.TrustedKeys)
			assert.Equal(t, filepath.Join(dir, "trust"), repo.Verification.KeysOrigin.Dir)
			assert.Equal(t, filepath.Join(dir, "store"), repo.Options["path"], "sibling settings keep the including file")
		})
	}
}

func TestLoadConfig_ArtifactsRemoteTrustedKeysIncludeIsNotAnchored(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "- release.pub\n")
	}))
	defer server.Close()
	for _, suffix := range []string{"", " | eval"} {
		t.Run(fmt.Sprintf("eval=%t", suffix != ""), func(t *testing.T) {
			dir := artifactsTestDir(t, "artifacts:\n  verification:\n    trusted_keys: !include "+server.URL+"/keys.yaml"+suffix+"\n")
			cfg, err := loadArtifactsDir(t, dir)
			require.NoError(t, err)
			require.NotNil(t, cfg.Artifacts.Verification)
			assert.Equal(t, []string{"release.pub"}, cfg.Artifacts.Verification.TrustedKeys, "a remote relative key is never anchored to a local directory")
			assert.True(t, cfg.Artifacts.Verification.KeysOrigin.Remote)

			err = artifactconfig.Validate(&cfg.Artifacts)
			require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			assert.Contains(t, err.Error(), "remote import")
		})
	}
}

func decodeYAMLForTest(t *testing.T, source string) (any, error) {
	t.Helper()
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(source), &node))
	return decodeNodeWithYamlFunctions(&node)
}

func TestDecodeNodeWithYamlFunctions_MergeKeys(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		want    map[string]any
		wantErr bool
	}{
		{
			name:   "alias of a mapping",
			source: "base: &base {a: 1, b: 2}\nchild:\n  <<: *base\n  b: 3\n  c: 4\n",
			want: map[string]any{
				"base":  map[string]any{"a": 1, "b": 2},
				"child": map[string]any{"a": 1, "b": 3, "c": 4},
			},
		},
		{
			name:   "own keys win regardless of order",
			source: "base: &base {a: 1}\nchild:\n  a: 9\n  <<: *base\n",
			want:   map[string]any{"base": map[string]any{"a": 1}, "child": map[string]any{"a": 9}},
		},
		{
			name:   "inline mapping",
			source: "child:\n  <<: {a: 1}\n  b: 2\n",
			want:   map[string]any{"child": map[string]any{"a": 1, "b": 2}},
		},
		{
			name:   "sequence of aliases: earlier sources win",
			source: "x: &x {a: 1, b: 1}\ny: &y {b: 2, c: 2}\nchild:\n  <<: [*x, *y]\n",
			want: map[string]any{
				"x":     map[string]any{"a": 1, "b": 1},
				"y":     map[string]any{"b": 2, "c": 2},
				"child": map[string]any{"a": 1, "b": 1, "c": 2},
			},
		},
		{name: "scalar merge value", source: "child:\n  <<: 5\n", wantErr: true},
		{name: "sequence with a scalar", source: "x: &x {a: 1}\nchild:\n  <<: [*x, 5]\n", wantErr: true},
		{
			name:   "quoted merge key is an ordinary key",
			source: "child:\n  \"<<\": 1\n",
			want:   map[string]any{"child": map[string]any{"<<": 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeYAMLForTest(t, tt.source)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrExecuteYamlFunctions)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDecodeNodeWithYamlFunctions_AliasPolicy(t *testing.T) {
	t.Run("an alias is a copy: mutating it leaves the anchor intact", func(t *testing.T) {
		got, err := decodeYAMLForTest(t, "base: &base {list: [1, 2]}\nref: *base\n")
		require.NoError(t, err)
		decoded, ok := got.(map[string]any)
		require.True(t, ok)
		ref, ok := decoded["ref"].(map[string]any)
		require.True(t, ok)
		ref["list"].([]any)[0] = 99
		ref["extra"] = true
		assert.Equal(t, map[string]any{"list": []any{1, 2}}, decoded["base"], "result to source isolation")

		base, ok := decoded["base"].(map[string]any)
		require.True(t, ok)
		base["list"].([]any)[1] = 77
		assert.Equal(t, []any{99, 2}, ref["list"], "source to result isolation")
	})

	t.Run("an alias to itself is a cycle, not a hang", func(t *testing.T) {
		_, err := decodeYAMLForTest(t, "loop: &loop [*loop]\n")
		require.ErrorIs(t, err, ErrExecuteYamlFunctions)
		assert.Contains(t, err.Error(), "cycle")
	})

	t.Run("an anchor outside the decoded subtree is evaluated on demand", func(t *testing.T) {
		var document yaml.Node
		require.NoError(t, yaml.Unmarshal([]byte("other: &shared {a: 1}\nsection:\n  ref: *shared\n"), &document))
		root := document.Content[0]
		section := root.Content[3]
		got, err := decodeNodeWithYamlFunctions(section)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"ref": map[string]any{"a": 1}}, got)
	})
}
