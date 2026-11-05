package projectroot

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errNoRepository = errors.New("not a repository")

// isolate clears every environment variable that influences discovery.
func isolate(t *testing.T) {
	t.Helper()
	for _, name := range []string{CliConfigPathEnvVar, GitRootBasePathEnvVar, testGitRootEnvVar, "ATMOS_CONFIG", "ATMOS_CONFIG_PATH", "ATMOS_BASE_PATH"} {
		t.Setenv(name, "")
	}
}

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()
	dir := filepath.Join(parts...)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	return dir
}

func touch(t *testing.T, parts ...string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(parts...), []byte("x"), 0o600))
}

func TestHasLocalConfig(t *testing.T) {
	for _, indicator := range []string{AtmosConfigFileName, DotAtmosConfigFileName} {
		dir := t.TempDir()
		assert.False(t, HasLocalConfig(dir))
		touch(t, dir, indicator)
		assert.True(t, HasLocalConfig(dir), indicator)
	}
	for _, indicator := range []string{AtmosConfigDirName, AtmosDefaultImportsDirName, DotAtmosDefaultImportsDirName} {
		dir := t.TempDir()
		mkdir(t, dir, indicator)
		assert.True(t, HasLocalConfig(dir), indicator)
	}
	assert.False(t, HasLocalConfig(filepath.Join(t.TempDir(), "missing")))
}

func TestSearchDirs(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	sub := mkdir(t, root, "components", "terraform", "mock")
	other := mkdir(t, t.TempDir(), "elsewhere")
	configFile := filepath.Join(other, "custom.yaml")
	touch(t, other, "custom.yaml")
	gitRootOf := func(dir string, err error) func() (string, error) {
		return func() (string, error) { return dir, err }
	}

	tests := []struct {
		name    string
		cwd     string
		args    []string
		env     map[string]string
		git     func() (string, error)
		prepare func(t *testing.T)
		want    []string
	}{
		{name: "git root fallback", cwd: sub, git: gitRootOf(root, nil), want: []string{sub, root}},
		{name: "git root disabled", cwd: sub, git: gitRootOf(root, nil), env: map[string]string{GitRootBasePathEnvVar: "false"}, want: []string{sub}},
		{name: "not in a repository", cwd: sub, git: gitRootOf("", errNoRepository), want: []string{sub}},
		{name: "git root same as cwd", cwd: root, git: gitRootOf(root, nil), want: []string{root}},
		{name: "relative git root", cwd: sub, git: gitRootOf(".", nil), want: []string{sub}},
		{
			name: "local config in cwd stops discovery", cwd: sub, git: gitRootOf(root, nil),
			prepare: func(t *testing.T) { touch(t, sub, AtmosConfigFileName) }, want: []string{sub},
		},
		{
			name: "cli config path dir", cwd: sub, git: gitRootOf(root, nil),
			env: map[string]string{CliConfigPathEnvVar: other}, want: []string{other, sub},
		},
		{
			name: "cli config path file", cwd: sub, git: gitRootOf(root, nil),
			env: map[string]string{CliConfigPathEnvVar: configFile}, want: []string{other, sub},
		},
		{
			name: "cli config path wins even with local config", cwd: sub, git: gitRootOf(root, nil),
			env: map[string]string{CliConfigPathEnvVar: other}, prepare: func(t *testing.T) { touch(t, sub, AtmosConfigFileName) },
			want: []string{other, sub},
		},
		{name: "flag --config-path", cwd: sub, git: gitRootOf(root, nil), args: []string{"atmos", "--config-path=" + other, "plan"}, want: []string{other, sub}},
		{name: "flag --config file", cwd: sub, git: gitRootOf(root, nil), args: []string{"atmos", "--config", configFile}, want: []string{other, sub}},
		{name: "flag beats ATMOS_CLI_CONFIG_PATH", cwd: sub, git: gitRootOf(root, nil), args: []string{"atmos", "--config-path=" + other}, env: map[string]string{CliConfigPathEnvVar: root}, want: []string{other, sub}},
		{name: "env ATMOS_CONFIG_PATH", cwd: sub, git: gitRootOf(root, nil), env: map[string]string{"ATMOS_CONFIG_PATH": other}, want: []string{other, sub}},
		{name: "env ATMOS_CONFIG", cwd: sub, git: gitRootOf(root, nil), env: map[string]string{"ATMOS_CONFIG": configFile}, want: []string{other, sub}},
		{name: "flag beats ATMOS_CONFIG_PATH", cwd: sub, git: gitRootOf(root, nil), args: []string{"atmos", "--config-path=" + root}, env: map[string]string{"ATMOS_CONFIG_PATH": other}, want: []string{root, sub}},
		{name: "relative --config-path resolves against cwd", cwd: sub, git: gitRootOf(root, nil), args: []string{"atmos", "--config-path=.."}, want: []string{filepath.Join(sub, ".."), sub}},
		{name: "flag selection ignores git root", cwd: sub, git: gitRootOf(root, nil), args: []string{"atmos", "--config-path=" + other}, want: []string{other, sub}},
		{
			name: "parent directory with atmos.yaml outside a repository", cwd: sub, git: gitRootOf("", errNoRepository),
			prepare: func(t *testing.T) { touch(t, root, AtmosConfigFileName) }, want: []string{sub, root},
		},
		{
			name: "parent directory with atmos.yaml inside a repository follows the git root", cwd: sub, git: gitRootOf(root, nil),
			prepare: func(t *testing.T) { touch(t, filepath.Join(root, "components"), AtmosConfigFileName) },
			want:    []string{sub, root, filepath.Join(root, "components")},
		},
		{
			name: "parent directory that is the git root is listed once", cwd: sub, git: gitRootOf(root, nil),
			prepare: func(t *testing.T) { touch(t, root, AtmosConfigFileName) }, want: []string{sub, root},
		},
		{
			name: "parent directory with atmos.yaml when git root disabled", cwd: sub, git: gitRootOf(root, nil),
			env: map[string]string{GitRootBasePathEnvVar: "false"}, prepare: func(t *testing.T) { touch(t, root, DotAtmosConfigFileName) },
			want: []string{sub, root},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			// Local-config fixtures are removed so cases stay independent.
			for _, dir := range []string{root, filepath.Join(root, "components"), sub} {
				for _, name := range []string{AtmosConfigFileName, DotAtmosConfigFileName} {
					require.NoError(t, removeIfExists(filepath.Join(dir, name)))
				}
			}
			for name, value := range tt.env {
				t.Setenv(name, value)
			}
			if tt.prepare != nil {
				tt.prepare(t)
			}
			previous := gitRootResolver
			gitRootResolver = tt.git
			t.Cleanup(func() { gitRootResolver = previous })

			assert.Equal(t, tt.want, SearchDirs(tt.cwd, tt.args))
		})
	}
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func TestSearchDirsRealRepository(t *testing.T) {
	isolate(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	_, err = gogit.PlainInit(root, false)
	require.NoError(t, err)
	sub := mkdir(t, root, "a", "b")
	t.Chdir(sub)
	cwd, err := os.Getwd()
	require.NoError(t, err)

	dirs := SearchDirs(cwd, nil)
	require.Len(t, dirs, 2)
	assert.Equal(t, cwd, dirs[0])
	resolved, err := filepath.EvalSymlinks(dirs[1])
	require.NoError(t, err)
	assert.Equal(t, root, resolved)
}

func TestTestGitRootOverride(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	t.Setenv(testGitRootEnvVar, root)
	got, err := defaultGitRoot()
	require.NoError(t, err)
	assert.Equal(t, root, got)
}

func TestFindConfigInParentDirs(t *testing.T) {
	root := t.TempDir()
	sub := mkdir(t, root, "a", "b")
	assert.Empty(t, FindConfigInParentDirs(sub), "no config in any ancestor below the temp root")
	touch(t, root, AtmosConfigFileName)
	assert.Equal(t, root, FindConfigInParentDirs(sub))
}

func TestConfigSelectionApplyFallbacks(t *testing.T) {
	sel := ConfigSelection{Config: []string{"flag.yaml"}}
	sel.ApplyFallbacks(ConfigSelection{BasePath: "env-base", Config: []string{"env.yaml"}, ConfigPath: []string{"env-dir"}})
	assert.Equal(t, ConfigSelection{BasePath: "env-base", Config: []string{"flag.yaml"}, ConfigPath: []string{"env-dir"}}, sel)
}
