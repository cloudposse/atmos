package config

import (
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// elsewhere returns an empty directory to use as the process working directory.
func elsewhere(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

func TestResolveConfigCandidates_WithWorkingDir(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, project string) (opts []CandidateOption, wantPath, wantSource string)
		gitEnabled bool
	}{
		{
			name: "working directory config is found without chdir",
			setup: func(t *testing.T, project string) ([]CandidateOption, string, string) {
				base := writeCandidateFile(t, project, "atmos.yaml")
				return []CandidateOption{WithWorkingDir(project)}, base, "working directory"
			},
		},
		{
			name: "parent directory config is found from a subdirectory",
			setup: func(t *testing.T, project string) ([]CandidateOption, string, string) {
				base := writeCandidateFile(t, project, "atmos.yaml")
				sub := filepath.Join(project, "stacks", "orgs")
				require.NoError(t, os.MkdirAll(sub, 0o755))
				return []CandidateOption{WithWorkingDir(sub)}, base, "parent directory"
			},
		},
		{
			name: "relative config file resolves against the working directory",
			setup: func(t *testing.T, project string) ([]CandidateOption, string, string) {
				base := writeCandidateFile(t, project, "custom.yaml")
				return []CandidateOption{WithWorkingDir(project), WithConfigFiles("custom.yaml")}, base, "--config"
			},
		},
		{
			name: "relative config directory resolves against the working directory",
			setup: func(t *testing.T, project string) ([]CandidateOption, string, string) {
				base := writeCandidateFile(t, project, "configs", "atmos.yaml")
				return []CandidateOption{WithWorkingDir(project), WithConfigDirs("configs")}, base, "--config-path"
			},
		},
		{
			name:       "git root config is found from a subdirectory",
			gitEnabled: true,
			setup: func(t *testing.T, project string) ([]CandidateOption, string, string) {
				_, err := gogit.PlainInit(project, false)
				require.NoError(t, err)
				base := writeCandidateFile(t, project, "atmos.yaml")
				sub := filepath.Join(project, "deep", "er")
				require.NoError(t, os.MkdirAll(sub, 0o755))
				return []CandidateOption{WithWorkingDir(sub)}, base, "git root"
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project := isolateCandidateEnv(t)
			if tt.gitEnabled {
				t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "true")
			}
			opts, wantPath, wantSource := tt.setup(t, project)
			other := elsewhere(t)
			t.Chdir(other)

			got := projectCandidates(t, opts...)

			require.Len(t, got, 1)
			assert.Equal(t, wantPath, got[0].Path)
			assert.Equal(t, wantSource, got[0].Source)
			assert.True(t, got[0].Exists)
			cwd, err := os.Getwd()
			require.NoError(t, err)
			resolvedCwd, err := filepath.EvalSymlinks(cwd)
			require.NoError(t, err)
			assert.Equal(t, other, resolvedCwd, "the process working directory must not change")
		})
	}
}

func TestResolveConfigCandidates_WithWorkingDir_NotAnchoredToProcessDirectory(t *testing.T) {
	project := isolateCandidateEnv(t)
	writeCandidateFile(t, project, "atmos.yaml")
	t.Chdir(project)
	empty := elsewhere(t)

	// The process directory has a config, but the requested working directory does not.
	got := projectCandidates(t, WithWorkingDir(empty))

	assert.Empty(t, got)
}

func TestResolveConfigCandidates_WithWorkingDir_RelativeDirectory(t *testing.T) {
	project := isolateCandidateEnv(t)
	base := writeCandidateFile(t, project, "nested", "atmos.yaml")
	t.Chdir(project)

	got := projectCandidates(t, WithWorkingDir("nested"))

	require.Len(t, got, 1)
	assert.Equal(t, base, got[0].Path)
}

func TestResolveBasePath_WorkingDir(t *testing.T) {
	root := isolateCandidateEnv(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub", "components"), 0o755))
	t.Chdir(elsewhere(t))

	tests := []struct {
		name, basePath, configDir, source string
		want                              string
	}{
		{name: "runtime dot path anchors to the working directory", basePath: "./sub", source: "runtime", want: filepath.Join(root, "sub")},
		{name: "config dot path anchors to the config directory", basePath: "./sub", configDir: root, want: filepath.Join(root, "sub")},
		{name: "runtime bare path falls back to the working directory", basePath: "sub", source: "runtime", want: filepath.Join(root, "sub")},
		{name: "absolute path is unchanged", basePath: filepath.Join(root, "sub"), want: filepath.Join(root, "sub")},
	}
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveBasePath(root, tt.basePath, tt.configDir, tt.source)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
