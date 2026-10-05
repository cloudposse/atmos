package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
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
// cleanup. EffectiveConfigFilesAscending probes the current directory for
// atmos.d/.atmos.d, so these tests must run from a known temp dir.
func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(dir))
}

func evalSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return resolved
}

// TestEffectiveConfigFilesAscending_RootIsHighestPrecedence confirms that the
// root atmos.yaml is ordered last (highest precedence), after the `.atmos.d/`
// fragments, matching the loader reapplying the root over its imports.
func TestEffectiveConfigFilesAscending_RootIsHighestPrecedence(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, AtmosConfigFileName)
	require.NoError(t, os.WriteFile(root, []byte("base_path: \"./\"\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".atmos.d"), 0o755))
	fragment := filepath.Join(dir, ".atmos.d", "mcp.yaml")
	require.NoError(t, os.WriteFile(fragment, []byte("mcp:\n  servers: {}\n"), 0o644))

	chdirForTest(t, dir)
	files := EffectiveConfigFilesAscending(nil)
	require.NotEmpty(t, files)

	resolved := make([]string, len(files))
	for i, f := range files {
		resolved[i] = evalSymlinks(t, f)
	}
	assert.Contains(t, resolved, evalSymlinks(t, fragment))
	// Root is last (highest precedence).
	assert.Equal(t, evalSymlinks(t, root), resolved[len(resolved)-1])
	assert.Less(t, indexOf(resolved, evalSymlinks(t, fragment)), len(resolved)-1, "fragment must precede the root")
}

// TestEffectiveConfigFilesAscending_FragmentOrderWithinDir confirms two fragments
// in the same `.atmos.d/` are ordered so the later (alphabetically-higher) file
// wins, matching the loader's later-overrides-earlier merge.
func TestEffectiveConfigFilesAscending_FragmentOrderWithinDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, AtmosConfigFileName), []byte("base_path: \"./\"\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".atmos.d"), 0o755))
	base := filepath.Join(dir, ".atmos.d", "00-base.yaml")
	local := filepath.Join(dir, ".atmos.d", "99-local.yaml")
	require.NoError(t, os.WriteFile(base, []byte("mcp:\n  servers: {}\n"), 0o644))
	require.NoError(t, os.WriteFile(local, []byte("mcp:\n  servers: {}\n"), 0o644))

	chdirForTest(t, dir)
	files := EffectiveConfigFilesAscending(nil)
	resolved := make([]string, len(files))
	for i, f := range files {
		resolved[i] = evalSymlinks(t, f)
	}
	assert.Less(t, indexOf(resolved, evalSymlinks(t, base)), indexOf(resolved, evalSymlinks(t, local)),
		"00-base must precede 99-local so the later file is highest precedence")
}

// TestEffectiveConfigFilesAscending_SkipsCWDWithoutRootConfig confirms the CWD
// atmos.d is excluded when the CWD has no root atmos.yaml of its own, because the
// loader would not merge it (cloudposse/atmos#3270 review).
func TestEffectiveConfigFilesAscending_SkipsCWDWithoutRootConfig(t *testing.T) {
	dir := t.TempDir()
	// No root atmos.yaml in the CWD, only a fragment.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".atmos.d"), 0o755))
	fragment := filepath.Join(dir, ".atmos.d", "mcp.yaml")
	require.NoError(t, os.WriteFile(fragment, []byte("mcp:\n  servers: {}\n"), 0o644))

	chdirForTest(t, dir)
	files := EffectiveConfigFilesAscending(nil)
	for _, f := range files {
		assert.NotEqual(t, evalSymlinks(t, fragment), evalSymlinks(t, f),
			"a CWD fragment must not be included when the CWD has no root atmos.yaml")
	}
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

// TestEffectiveConfigFilesAscending_GitRootAndCWD exercises the git-root branch of
// fragmentDirsAscending: from a subdirectory that has its own root atmos.yaml, both
// the git-root `.atmos.d` fragments and the CWD `.atmos.d` fragments participate,
// with the git-root fragment at lower precedence (earlier) than the CWD fragment.
func TestEffectiveConfigFilesAscending_GitRootAndCWD(t *testing.T) {
	repo := t.TempDir()
	_, err := git.PlainInit(repo, false)
	require.NoError(t, err)

	rootFragment := filepath.Join(repo, ".atmos.d", "mcp.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(rootFragment), 0o755))
	require.NoError(t, os.WriteFile(rootFragment, []byte("mcp:\n  servers: {}\n"), 0o644))

	sub := filepath.Join(repo, "project")
	require.NoError(t, os.MkdirAll(filepath.Join(sub, ".atmos.d"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sub, AtmosConfigFileName), []byte("base_path: \"./\"\n"), 0o644))
	subFragment := filepath.Join(sub, ".atmos.d", "mcp.yaml")
	require.NoError(t, os.WriteFile(subFragment, []byte("mcp:\n  servers: {}\n"), 0o644))

	chdirForTest(t, sub)
	files := EffectiveConfigFilesAscending(nil)
	resolved := make([]string, len(files))
	for i, f := range files {
		resolved[i] = evalSymlinks(t, f)
	}

	rootIdx := indexOf(resolved, evalSymlinks(t, rootFragment))
	subIdx := indexOf(resolved, evalSymlinks(t, subFragment))
	require.GreaterOrEqual(t, rootIdx, 0, "git-root fragment must be included")
	require.GreaterOrEqual(t, subIdx, 0, "CWD fragment must be included")
	assert.Less(t, rootIdx, subIdx, "git-root fragment must have lower precedence than the CWD fragment")
}

// TestEffectiveConfigFilesAscending_IncludesActiveProfile covers the #3270 review
// gap: an active profile's files participate in the merge at the highest precedence
// (after the root atmos.yaml), so they must appear last in the ascending list.
func TestEffectiveConfigFilesAscending_IncludesActiveProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("ATMOS_PROFILE", "")

	dir := t.TempDir()
	root := filepath.Join(dir, AtmosConfigFileName)
	require.NoError(t, os.WriteFile(root, []byte("base_path: \"./\"\n"), 0o644))
	profileFile := filepath.Join(dir, "profiles", "dev", "mcp.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(profileFile), 0o755))
	require.NoError(t, os.WriteFile(profileFile, []byte("mcp:\n  servers: {}\n"), 0o644))

	chdirForTest(t, dir)
	atmosConfig := &schema.AtmosConfiguration{CliConfigPath: dir}
	atmosConfig.Profiles.Default = "dev"

	files := EffectiveConfigFilesAscending(atmosConfig)
	require.NotEmpty(t, files)
	resolved := make([]string, len(files))
	for i, f := range files {
		resolved[i] = evalSymlinks(t, f)
	}
	assert.Equal(t, evalSymlinks(t, profileFile), resolved[len(resolved)-1],
		"active profile file must be highest precedence (last in ascending order)")
	rootIdx := indexOf(resolved, evalSymlinks(t, root))
	profIdx := indexOf(resolved, evalSymlinks(t, profileFile))
	require.GreaterOrEqual(t, rootIdx, 0)
	assert.Less(t, rootIdx, profIdx, "root atmos.yaml must have lower precedence than an active profile")
}

// TestActiveProfileFilesNilConfig confirms nil atmosConfig is handled (no panic).
func TestActiveProfileFilesNilConfig(t *testing.T) {
	assert.Nil(t, activeProfileFiles(nil))
}

// TestFragmentFilesSkipsNonDirImportPath covers fragmentFiles when an import name
// exists as a file rather than a directory: it must be skipped, and the real
// `.atmos.d/` directory still searched.
func TestFragmentFilesSkipsNonDirImportPath(t *testing.T) {
	dir := t.TempDir()
	// `atmos.d` is a FILE here, not a directory.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.d"), []byte("ignored\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".atmos.d"), 0o755))
	fragment := filepath.Join(dir, ".atmos.d", "mcp.yaml")
	require.NoError(t, os.WriteFile(fragment, []byte("mcp:\n  servers: {}\n"), 0o644))

	files := fragmentFiles(dir)
	resolved := make([]string, len(files))
	for i, f := range files {
		resolved[i] = evalSymlinks(t, f)
	}
	assert.Contains(t, resolved, evalSymlinks(t, fragment))
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
