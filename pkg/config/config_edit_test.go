package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveEditableConfigFile_Override(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "custom.yaml")
	require.NoError(t, os.WriteFile(file, []byte("a: 1\n"), 0o644))

	got, err := ResolveEditableConfigFile(nil, file)
	require.NoError(t, err)
	assert.Equal(t, file, got)
}

func TestResolveEditableConfigFile_OverrideDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, AtmosConfigFileName)
	require.NoError(t, os.WriteFile(file, []byte("a: 1\n"), 0o644))

	got, err := ResolveEditableConfigFile(nil, dir)
	require.NoError(t, err)
	assert.Equal(t, file, got)
}

func TestResolveEditableConfigFile_OverrideDirWithoutConfig(t *testing.T) {
	_, err := ResolveEditableConfigFile(nil, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoEditableConfig)
}

func TestResolveEditableConfigFile_OverrideMissing(t *testing.T) {
	_, err := ResolveEditableConfigFile(nil, filepath.Join(t.TempDir(), "nope.yaml"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoEditableConfig)
}

func TestResolveEditableConfigFile_PrefersAtmosYamlOverDotfile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, AtmosConfigFileName), []byte("a: 1\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, DotAtmosConfigFileName), []byte("a: 2\n"), 0o644))

	got, ok, err := firstExistingConfig(dir)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, filepath.Join(dir, AtmosConfigFileName), got)
}

func TestResolveEditableConfigFile_NoCurrentDirectoryConfig(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(dir))

	_, err = ResolveEditableConfigFile(nil, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoEditableConfig)
}

// TestResolveEditableConfigFile_NonMissingStatErrorPropagates verifies that a
// stat error other than "not found" surfaces instead of collapsing into
// ErrNoEditableConfig. Using a regular file as a directory component yields
// ENOTDIR on Unix, which os.IsNotExist does not match.
func TestResolveEditableConfigFile_NonMissingStatErrorPropagates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports a file-as-directory probe as path-not-found, which os.IsNotExist treats as missing")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))

	override := filepath.Join(file, AtmosConfigFileName)
	_, err := ResolveEditableConfigFile(nil, override)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNoEditableConfig, "ENOTDIR must not be reported as missing config")
}

func TestResolveEditableConfigFile_CurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, AtmosConfigFileName)
	require.NoError(t, os.WriteFile(file, []byte("a: 1\n"), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(dir))

	got, err := ResolveEditableConfigFile(nil, "")
	require.NoError(t, err)
	// Resolve symlinks for macOS /var -> /private/var.
	gotResolved, _ := filepath.EvalSymlinks(got)
	wantResolved, _ := filepath.EvalSymlinks(file)
	assert.Equal(t, wantResolved, gotResolved)
}

// chdirForTest switches to dir and restores the previous working directory on
// cleanup. The fragment-aware resolver probes the current directory for
// atmos.d/.atmos.d, so these tests must run from a known temp dir.
func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(dir))
}

func TestFileDeclaresTopLevelKey(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
		return p
	}

	tests := []struct {
		name    string
		path    string
		key     string
		want    bool
		wantErr bool
	}{
		{name: "declares key", path: write("has.yaml", "mcp:\n  enabled: true\n"), key: "mcp", want: true},
		{name: "declares key with null value", path: write("null.yaml", "mcp:\n"), key: "mcp", want: true},
		{name: "does not declare key", path: write("other.yaml", "commands: []\n"), key: "mcp", want: false},
		{name: "empty file", path: write("empty.yaml", ""), key: "mcp", want: false},
		{name: "scalar root is not a mapping", path: write("scalar.yaml", "42\n"), key: "mcp", want: false},
		{name: "missing file errors", path: filepath.Join(dir, "nope.yaml"), key: "mcp", wantErr: true},
		{name: "invalid yaml errors", path: write("bad.yaml", "mcp: [unterminated\n"), key: "mcp", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := fileDeclaresTopLevelKey(tt.path, tt.key)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestResolveEditableConfigFileForSection_PrefersFragment covers the #3269 fix:
// both the `atmos.d/` and `.atmos.d/` directory variants, a nested fragment, and
// a `.yml` extension must be discovered and preferred over the root atmos.yaml.
func TestResolveEditableConfigFileForSection_PrefersFragment(t *testing.T) {
	tests := []struct {
		name         string
		fragmentRel  string
		fragmentBody string
	}{
		{name: "dot atmos.d yaml", fragmentRel: filepath.Join(".atmos.d", "mcp.yaml"), fragmentBody: "mcp:\n  servers: {}\n"},
		{name: "atmos.d yaml", fragmentRel: filepath.Join("atmos.d", "mcp.yaml"), fragmentBody: "mcp:\n  enabled: true\n"},
		{name: "nested fragment", fragmentRel: filepath.Join(".atmos.d", "nested", "mcp.yaml"), fragmentBody: "mcp:\n  servers: {}\n"},
		{name: "yml extension", fragmentRel: filepath.Join(".atmos.d", "mcp.yml"), fragmentBody: "mcp:\n  servers: {}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, AtmosConfigFileName), []byte("base_path: \"./\"\n"), 0o644))
			fragment := filepath.Join(dir, tt.fragmentRel)
			require.NoError(t, os.MkdirAll(filepath.Dir(fragment), 0o755))
			require.NoError(t, os.WriteFile(fragment, []byte(tt.fragmentBody), 0o644))

			chdirForTest(t, dir)
			got, err := ResolveEditableConfigFileForSection(nil, "", "mcp")
			require.NoError(t, err)

			wantResolved, _ := filepath.EvalSymlinks(fragment)
			gotResolved, _ := filepath.EvalSymlinks(got)
			assert.Equal(t, wantResolved, gotResolved)
		})
	}
}

func TestResolveEditableConfigFileForSection_FallsBackToRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, AtmosConfigFileName)
	require.NoError(t, os.WriteFile(root, []byte("base_path: \"./\"\n"), 0o644))
	// A fragment that declares an unrelated section must not hijack the target.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".atmos.d"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".atmos.d", "commands.yaml"), []byte("commands: []\n"), 0o644))

	chdirForTest(t, dir)
	got, err := ResolveEditableConfigFileForSection(nil, "", "mcp")
	require.NoError(t, err)

	wantResolved, _ := filepath.EvalSymlinks(root)
	gotResolved, _ := filepath.EvalSymlinks(got)
	assert.Equal(t, wantResolved, gotResolved)
}

func TestResolveEditableConfigFileForSection_OverrideWins(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, AtmosConfigFileName), []byte("base_path: \"./\"\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".atmos.d"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".atmos.d", "mcp.yaml"), []byte("mcp:\n  servers: {}\n"), 0o644))
	override := filepath.Join(dir, "custom.yaml")
	require.NoError(t, os.WriteFile(override, []byte("a: 1\n"), 0o644))

	chdirForTest(t, dir)
	got, err := ResolveEditableConfigFileForSection(nil, override, "mcp")
	require.NoError(t, err)
	assert.Equal(t, override, got)
}

// TestResolveEditableConfigFileForSection_NoConfigAtAll confirms the error path
// when neither a fragment nor a root atmos.yaml exists.
func TestResolveEditableConfigFileForSection_NoConfigAtAll(t *testing.T) {
	chdirForTest(t, t.TempDir())
	_, err := ResolveEditableConfigFileForSection(nil, "", "mcp")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoEditableConfig)
}

// TestResolveConfigOverride covers all three branches: zero, one, and multiple --config files.
func TestResolveConfigOverride(t *testing.T) {
	tests := []struct {
		name     string
		cfgFiles []string
		want     string
		wantErr  bool
	}{
		{name: "no files", cfgFiles: nil, want: ""},
		{name: "single file", cfgFiles: []string{"a.yaml"}, want: "a.yaml"},
		{name: "multiple files are ambiguous", cfgFiles: []string{"a.yaml", "b.yaml"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveConfigOverride(tt.cfgFiles)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrAmbiguousConfigFile)
				assert.Contains(t, err.Error(), "a.yaml")
				assert.Contains(t, err.Error(), "b.yaml")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
