package validation

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	ecconfig "github.com/editorconfig-checker/editorconfig-checker/v3/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields these tests rely on.
var _ = schema.EditorConfig{IgnoreDefaults: true, Exclude: []string{"x"}, DisableIndentation: true}

func editorConfigRelative(t *testing.T, root string, paths []string) []string {
	t.Helper()
	var relative []string
	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		require.NoError(t, err)
		relative = append(relative, filepath.ToSlash(rel))
	}
	sort.Strings(relative)
	return relative
}

func newEditorConfigConfig() *ecconfig.Config {
	return ecconfig.NewConfig(nil)
}

func TestEditorConfigFiles_GitSelection(t *testing.T) {
	root := newAffectedRepo(t)
	writeRepoFile(t, root, ".gitignore", ".workdir/\n")
	writeRepoFile(t, root, ".workdir/vendored/ignored.txt", "x\n")
	writeRepoFile(t, root, "untracked.txt", "x\n")
	writeRepoFile(t, root, "vendor/lib/main.tf", "x\n")
	writeRepoFile(t, root, "run.log", "x\n")
	gitInRepo(t, root, "add", "vendor/lib/main.tf")

	paths, err := EditorConfigFiles(root, newEditorConfigConfig(), nil)
	require.NoError(t, err)

	got := editorConfigRelative(t, root, paths)
	assert.Equal(t, []string{".gitignore", "base.yaml", "stacks/keep.yaml", "untracked.txt", "vendor/lib/main.tf"}, got,
		"ignored paths and default-excluded log files must not be listed")
	for _, path := range paths {
		assert.True(t, filepath.IsAbs(path), "an absolute root yields absolute paths: %s", path)
	}
}

func TestEditorConfigFiles_GlobExcludesAreRootRelative(t *testing.T) {
	root := newAffectedRepo(t)
	writeRepoFile(t, root, "vendor/lib/main.tf", "x\n")
	// The working directory is deliberately not the root: patterns must match root-relative paths.
	t.Chdir(t.TempDir())

	paths, err := EditorConfigFiles(root, newEditorConfigConfig(), []string{"vendor/**", "stacks/**"})
	require.NoError(t, err)
	assert.Equal(t, []string{"base.yaml"}, editorConfigRelative(t, root, paths))

	_, err = EditorConfigFiles(root, newEditorConfigConfig(), []string{"["})
	require.Error(t, err, "an invalid glob is reported, never ignored")
}

func TestEditorConfigFiles_RegexExcludesAndIgnoreDefaults(t *testing.T) {
	root := newAffectedRepo(t)
	writeRepoFile(t, root, "run.log", "x\n")
	writeRepoFile(t, root, "generated/out.txt", "x\n")

	cfg := newEditorConfigConfig()
	cfg.Exclude = []string{"^generated/"}
	paths, err := EditorConfigFiles(root, cfg, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"base.yaml", "stacks/keep.yaml"}, editorConfigRelative(t, root, paths))

	cfg = newEditorConfigConfig()
	cfg.IgnoreDefaults = true
	paths, err = EditorConfigFiles(root, cfg, nil)
	require.NoError(t, err)
	assert.Contains(t, editorConfigRelative(t, root, paths), "run.log", "ignore_defaults keeps files the defaults skip")
}

func TestEditorConfigFiles_WalksOutsideGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	writeRepoFile(t, root, "a.txt", "x\n")
	writeRepoFile(t, root, "sub/b.txt", "x\n")
	writeRepoFile(t, root, "node_modules/pkg/index.js", "x\n")
	writeRepoFile(t, root, "run.log", "x\n")
	cfg := newEditorConfigConfig()
	cfg.Exclude = []string{"^sub/skipme"}
	writeRepoFile(t, root, "sub/skipme/c.txt", "x\n")

	paths, err := EditorConfigFiles(root, cfg, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"a.txt", "node_modules/pkg/index.js", "sub/b.txt"}, editorConfigRelative(t, root, paths),
		"node_modules is only a default exclude when nested below the root; directories matching an exclude are pruned")
}

func TestEditorConfigFiles_RelativeRootYieldsRelativePaths(t *testing.T) {
	root := newAffectedRepo(t)
	t.Chdir(root)

	paths, err := EditorConfigFiles(".", newEditorConfigConfig(), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"base.yaml", filepath.Join("stacks", "keep.yaml")}, sortedCopy(paths))
}

func sortedCopy(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

func TestEditorConfigFiles_SkipsDeletedAndBinaryFiles(t *testing.T) {
	root := newAffectedRepo(t)
	writeRepoFile(t, root, "doc.dat", "%PDF-1.4\n1 0 obj\n")
	require.NoError(t, os.Remove(filepath.Join(root, "base.yaml")))

	paths, err := EditorConfigFiles(root, newEditorConfigConfig(), nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"stacks/keep.yaml"}, editorConfigRelative(t, root, paths),
		"a file deleted from the work tree and a non-text file are skipped")
}

func TestSelectEditorConfigFiles(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt")

	assert.Equal(t, []string{a, b}, SelectEditorConfigFiles([]string{a, b}, nil), "a nil selection keeps everything")
	assert.Equal(t, []string{b}, SelectEditorConfigFiles([]string{a, b}, []string{b}))
	assert.Empty(t, SelectEditorConfigFiles([]string{a, b}, []string{}), "an empty selection keeps nothing")
}

func TestApplyEditorConfigSettings(t *testing.T) {
	cfg := newEditorConfigConfig()
	settings := schema.EditorConfig{IgnoreDefaults: true, DisableIndentation: true, DisableMaxLineLength: true}

	ApplyEditorConfigSettings(cfg, &settings)

	assert.True(t, cfg.IgnoreDefaults)
	assert.True(t, cfg.Disable.Indentation)
	assert.True(t, cfg.Disable.MaxLineLength)
	assert.False(t, cfg.Disable.EndOfLine, "unset settings never switch a check off")

	preset := newEditorConfigConfig()
	preset.Disable.EndOfLine = true
	ApplyEditorConfigSettings(preset, &schema.EditorConfig{})
	assert.True(t, preset.Disable.EndOfLine, "absent settings never re-enable a check")
}

func TestEditorConfigSettingsFromMap(t *testing.T) {
	settings, err := EditorConfigSettingsFromMap(map[string]any{
		"validate": map[string]any{"editorconfig": map[string]any{
			"ignore_defaults": true, "exclude": []any{"vendor/**"}, "disable_end_of_line": true,
		}},
	})
	require.NoError(t, err)
	assert.True(t, settings.IgnoreDefaults)
	assert.True(t, settings.DisableEndOfLine)
	assert.Equal(t, []string{"vendor/**"}, settings.Exclude)

	for _, config := range []map[string]any{nil, {}, {"validate": map[string]any{}}, {"validate": map[string]any{"editorconfig": nil}}} {
		settings, err = EditorConfigSettingsFromMap(config)
		require.NoError(t, err)
		assert.Equal(t, schema.EditorConfig{}, settings)
	}

	_, err = EditorConfigSettingsFromMap(map[string]any{"validate": map[string]any{"editorconfig": map[string]any{"exclude": "not-a-list"}}})
	require.Error(t, err)
}
