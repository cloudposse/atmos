package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// isolateCandidateEnv removes ambient inputs (home, XDG, env overrides, git root) so tests only see their fixtures.
func isolateCandidateEnv(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	home := filepath.Join(root, "home")
	require.NoError(t, os.MkdirAll(home, 0o755))
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// The home directory lookup is cached process-wide; reset it so the new HOME is seen.
	homedir.Reset()
	t.Cleanup(homedir.Reset)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	t.Setenv("ATMOS_CONFIG", "")
	t.Setenv("ATMOS_CONFIG_PATH", "")
	t.Setenv("ATMOS_PROFILE", "")
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
	t.Setenv("TEST_GIT_ROOT", "")
	project := filepath.Join(root, "project")
	require.NoError(t, os.MkdirAll(project, 0o755))
	return project
}

func writeCandidateFile(t *testing.T, elems ...string) string {
	t.Helper()
	path := filepath.Join(elems...)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("base_path: \".\"\n"), 0o644))
	return path
}

// projectCandidates returns the non-ambient candidates (a developer machine may have system-wide config).
func projectCandidates(t *testing.T, opts ...CandidateOption) []ConfigCandidate {
	t.Helper()
	all, err := ResolveConfigCandidates(opts...)
	require.NoError(t, err)
	var out []ConfigCandidate
	for _, c := range all {
		if c.Kind != ConfigCandidateAmbient {
			out = append(out, c)
		}
	}
	return out
}

func candidatePaths(cs []ConfigCandidate) []string {
	paths := make([]string, 0, len(cs))
	for _, c := range cs {
		paths = append(paths, c.Path)
	}
	return paths
}

func TestResolveConfigCandidates_SubdirectoryFindsParentConfig(t *testing.T) {
	project := isolateCandidateEnv(t)
	base := writeCandidateFile(t, project, "atmos.yaml")
	sub := filepath.Join(project, "stacks", "orgs")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	t.Chdir(sub)

	got := projectCandidates(t)
	require.Len(t, got, 1)
	assert.Equal(t, base, got[0].Path)
	assert.Equal(t, ConfigCandidateBase, got[0].Kind)
	assert.Equal(t, "parent directory", got[0].Source)
	assert.True(t, got[0].Exists)
}

func TestResolveConfigCandidates_NoConfigFound(t *testing.T) {
	project := isolateCandidateEnv(t)
	t.Chdir(project)

	assert.Empty(t, projectCandidates(t))
}

func TestResolveConfigCandidates_WorkingDirectory(t *testing.T) {
	project := isolateCandidateEnv(t)
	base := writeCandidateFile(t, project, "atmos.yaml")
	t.Chdir(project)

	got := projectCandidates(t)
	require.Len(t, got, 1)
	assert.Equal(t, base, got[0].Path)
	assert.Equal(t, "working directory", got[0].Source)
}

func TestResolveConfigCandidates_EnvConfigPathOverridesDiscovery(t *testing.T) {
	project := isolateCandidateEnv(t)
	parentConfig := writeCandidateFile(t, project, "atmos.yaml")
	envDir := filepath.Join(filepath.Dir(project), "elsewhere")
	envConfig := writeCandidateFile(t, envDir, "atmos.yaml")
	sub := filepath.Join(project, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	t.Chdir(sub)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", envDir)

	got := projectCandidates(t)
	require.Len(t, got, 1, "parent discovery is disabled by ATMOS_CLI_CONFIG_PATH: %v", candidatePaths(got))
	assert.Equal(t, envConfig, got[0].Path)
	assert.Equal(t, AtmosCliConfigPathEnvVar, got[0].Source)
	assert.NotContains(t, candidatePaths(got), parentConfig)
}

func TestResolveConfigCandidates_EnvConfigPathMissingIsReported(t *testing.T) {
	project := isolateCandidateEnv(t)
	t.Chdir(project)
	missing := filepath.Join(project, "nope")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", missing)

	got := projectCandidates(t)
	require.Len(t, got, 1)
	assert.False(t, got[0].Exists)
	assert.Equal(t, filepath.Join(missing, "atmos.yaml"), got[0].Path)
}

// The legacy single CLI config directory (ConfigAndStacksInfo.AtmosCliConfigPath) is resolved
// relative to the working directory and layered after the discovered configuration, so it takes
// precedence without turning discovery off. A missing directory is reported, not skipped.
func TestResolveConfigCandidates_CliConfigPathLayersOverDiscovery(t *testing.T) {
	project := isolateCandidateEnv(t)
	parentConfig := writeCandidateFile(t, project, "atmos.yaml")
	cliConfig := writeCandidateFile(t, project, "conf", "atmos.yaml")
	sub := filepath.Join(project, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	t.Chdir(sub)

	got := projectCandidates(t, WithCliConfigPath(filepath.Join("..", "conf")))
	require.Len(t, got, 2, "%v", candidatePaths(got))
	assert.Equal(t, parentConfig, got[0].Path)
	assert.Equal(t, cliConfig, got[1].Path, "the CLI config path is applied last")
	assert.Equal(t, "CLI config path", got[1].Source)
	assert.True(t, got[1].Exists)

	missing := projectCandidates(t, WithCliConfigPath(filepath.Join(project, "nope")))
	require.Len(t, missing, 2, "%v", candidatePaths(missing))
	assert.Equal(t, parentConfig, missing[0].Path)
	assert.Equal(t, filepath.Join(project, "nope", "atmos.yaml"), missing[1].Path)
	assert.False(t, missing[1].Exists, "a missing CLI config path is reported, not skipped")
}

func TestResolveConfigCandidates_AtmosDRelativeToConfigDirNotCwd(t *testing.T) {
	project := isolateCandidateEnv(t)
	base := writeCandidateFile(t, project, "atmos.yaml")
	a := writeCandidateFile(t, project, "atmos.d", "a.yaml")
	nested := writeCandidateFile(t, project, "atmos.d", "nested", "n.yml")
	b := writeCandidateFile(t, project, ".atmos.d", "b.yaml")
	sub := filepath.Join(project, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	t.Chdir(sub)

	got := projectCandidates(t)
	// atmos.d files precede the base file, which is applied last.
	require.Equal(t, []string{a, nested, b, base}, candidatePaths(got))
	assert.Equal(t, ConfigCandidateAtmosD, got[0].Kind)
	assert.Equal(t, "atmos.d", got[0].Source)
	assert.Equal(t, ".atmos.d", got[2].Source)
	assert.Equal(t, ConfigCandidateBase, got[3].Kind)
}

func TestResolveConfigCandidates_GitRootAtmosDLoadedBeforeConfigDir(t *testing.T) {
	project := isolateCandidateEnv(t)
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "")
	t.Setenv("TEST_GIT_ROOT", project)
	cfgDir := filepath.Join(project, "infra")
	base := writeCandidateFile(t, cfgDir, "atmos.yaml")
	rootD := writeCandidateFile(t, project, ".atmos.d", "root.yaml")
	localD := writeCandidateFile(t, cfgDir, "atmos.d", "local.yaml")
	t.Chdir(cfgDir)

	got := projectCandidates(t)
	require.Equal(t, []string{rootD, localD, base}, candidatePaths(got))
	assert.Equal(t, ".atmos.d (git root)", got[0].Source)
}

func TestResolveConfigCandidates_GitRootConfig(t *testing.T) {
	project := isolateCandidateEnv(t)
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "")
	t.Setenv("TEST_GIT_ROOT", project)
	base := writeCandidateFile(t, project, "atmos.yaml")
	sub := filepath.Join(project, "deep", "er")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	t.Chdir(sub)

	got := projectCandidates(t)
	// Parent search and git root both find the same file: it is reported once.
	require.Len(t, got, 1)
	assert.Equal(t, base, got[0].Path)
}

func TestResolveConfigCandidates_OnlyActiveProfiles(t *testing.T) {
	project := isolateCandidateEnv(t)
	base := writeCandidateFile(t, project, "atmos.yaml")
	ci := writeCandidateFile(t, project, ".atmos", "profiles", "ci", "settings.yaml")
	ciNested := writeCandidateFile(t, project, ".atmos", "profiles", "ci", "auth", "id.yml")
	writeCandidateFile(t, project, ".atmos", "profiles", "dev", "settings.yaml")
	t.Chdir(project)

	got := projectCandidates(t, WithProfiles("ci"))
	require.Equal(t, base, got[0].Path)
	profilePaths := candidatePaths(got[1:])
	assert.ElementsMatch(t, []string{ci, ciNested}, profilePaths)
	for _, c := range got[1:] {
		assert.Equal(t, ConfigCandidateProfile, c.Kind)
		assert.Equal(t, "ci", c.Profile)
		assert.True(t, c.Exists)
	}
	assert.NotContains(t, profilePaths, filepath.Join(project, ".atmos", "profiles", "dev", "settings.yaml"))
}

func TestResolveConfigCandidates_ProfilePrecedenceAndBasePath(t *testing.T) {
	project := isolateCandidateEnv(t)
	writeCandidateFile(t, project, "atmos.yaml")
	custom := writeCandidateFile(t, project, "custom-profiles", "ci", "c.yaml")
	writeCandidateFile(t, project, "profiles", "ci", "plain.yaml")
	t.Chdir(project)

	got := projectCandidates(t, WithProfiles("ci"), WithProfilesBasePath("custom-profiles", project))
	require.Len(t, got, 2)
	assert.Equal(t, custom, got[1].Path, "profiles.base_path wins over project profiles/")
}

func TestResolveConfigCandidates_MissingProfileReportedAsNonExistent(t *testing.T) {
	project := isolateCandidateEnv(t)
	writeCandidateFile(t, project, "atmos.yaml")
	t.Chdir(project)

	got := projectCandidates(t, WithProfiles("ghost", ProfileFlagSelectValue))
	require.Len(t, got, 2)
	assert.Equal(t, ConfigCandidateProfile, got[1].Kind)
	assert.Equal(t, "ghost", got[1].Profile)
	assert.False(t, got[1].Exists)
}

func TestResolveConfigCandidates_ConfigFilesSkipDiscoveryAndAtmosD(t *testing.T) {
	project := isolateCandidateEnv(t)
	writeCandidateFile(t, project, "atmos.yaml")
	other := writeCandidateFile(t, project, "conf", "extra.yaml")
	writeCandidateFile(t, project, "conf", "atmos.d", "x.yaml")
	t.Chdir(project)

	got := projectCandidates(t, WithConfigFiles(filepath.Join("conf", "extra.yaml"), filepath.Join("conf", "missing.yaml")))
	require.Len(t, got, 2)
	assert.Equal(t, other, got[0].Path)
	assert.True(t, got[0].Exists)
	assert.Equal(t, "--config", got[0].Source)
	assert.False(t, got[1].Exists)
}

func TestResolveConfigCandidates_ConfigDirsIncludeAtmosD(t *testing.T) {
	project := isolateCandidateEnv(t)
	writeCandidateFile(t, project, "atmos.yaml")
	dir := filepath.Join(project, "conf")
	base := writeCandidateFile(t, dir, "atmos.yaml")
	d := writeCandidateFile(t, dir, "atmos.d", "x.yaml")
	t.Chdir(project)

	got := projectCandidates(t, WithConfigDirs("conf"))
	require.Equal(t, []string{d, base}, candidatePaths(got))
}

func TestResolveConfigCandidates_ConfigSelectionFromEnv(t *testing.T) {
	project := isolateCandidateEnv(t)
	writeCandidateFile(t, project, "atmos.yaml")
	other := writeCandidateFile(t, project, "conf", "atmos.yaml")
	t.Chdir(project)
	t.Setenv("ATMOS_CONFIG_PATH", "conf")

	got := projectCandidates(t)
	require.Len(t, got, 1)
	assert.Equal(t, other, got[0].Path)
}

func TestResolveConfigCandidates_HomeIsAmbient(t *testing.T) {
	project := isolateCandidateEnv(t)
	home := writeCandidateFile(t, filepath.Dir(project), "home", ".atmos", "atmos.yaml")
	t.Chdir(project)

	all, err := ResolveConfigCandidates()
	require.NoError(t, err)
	var found bool
	for _, c := range all {
		if c.Path == home {
			found = true
			assert.Equal(t, ConfigCandidateAmbient, c.Kind)
			assert.Equal(t, "home directory", c.Source)
		}
	}
	assert.True(t, found, "home config must be reported as ambient: %v", all)
	assert.Empty(t, projectCandidates(t))
}

func TestResolveConfigCandidates_MatchesLoader(t *testing.T) {
	scenarios := []struct {
		name     string
		profiles []string
		setup    func(t *testing.T, project string) string // returns cwd.
	}{
		{
			name: "subdirectory with atmos.d",
			setup: func(t *testing.T, project string) string {
				writeCandidateFile(t, project, "atmos.yaml")
				writeCandidateFile(t, project, "atmos.d", "a.yaml")
				writeCandidateFile(t, project, ".atmos.d", "b.yaml")
				sub := filepath.Join(project, "sub")
				require.NoError(t, os.MkdirAll(sub, 0o755))
				return sub
			},
		},
		{
			name:     "active profile only",
			profiles: []string{"ci"},
			setup: func(t *testing.T, project string) string {
				writeCandidateFile(t, project, "atmos.yaml")
				writeCandidateFile(t, project, ".atmos", "profiles", "ci", "settings.yaml")
				writeCandidateFile(t, project, ".atmos", "profiles", "dev", "settings.yaml")
				return project
			},
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			project := isolateCandidateEnv(t)
			cwd := sc.setup(t, project)
			t.Chdir(cwd)

			info := schema.ConfigAndStacksInfo{ProfilesFromArg: sc.profiles}
			_, err := LoadConfig(&info)
			require.NoError(t, err)
			loaded := LoadedConfigFiles()

			all, err := ResolveConfigCandidates(WithProfiles(sc.profiles...))
			require.NoError(t, err)
			var want []string
			for _, c := range all {
				if c.Exists {
					want = append(want, c.Path)
				}
			}
			assert.ElementsMatch(t, loaded, want, "resolver must report exactly the files the loader merged")
		})
	}
}

func TestMissingProfileWithoutLocations(t *testing.T) {
	wd := t.TempDir()
	r := candidateResolver{wd: wd}
	require.NotPanics(t, func() { r.addProfile("missing", nil) })
	require.Len(t, r.candidates, 1)
	require.Equal(t, filepath.Join(wd, ".atmos", "profiles", "missing"), r.candidates[0].Path)
	require.False(t, r.candidates[0].Exists)
}
