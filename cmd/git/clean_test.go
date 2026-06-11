package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRunCleanRequiresRepositoryNameOrAll(t *testing.T) {
	err := runClean(context.Background(), &cleanOptions{Force: true}, nil)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrGitRepositoryRequired))
}

func TestRunCleanNoArgUsesSingleConfiguredRepository(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", filepath.Join(root, "cache"))
	withGitCleanConfig(t, root, map[string]schema.GitRepository{
		"deploy": {URI: "https://github.com/acme/deploy.git"},
	})

	err := runClean(context.Background(), &cleanOptions{DryRun: true}, nil)

	require.NoError(t, err)
}

func TestRunCleanDryRunDoesNotDelete(t *testing.T) {
	requireGit(t)

	root := t.TempDir()
	remote := initTestRemote(t, root)
	workdir := filepath.Join(root, "workdirs", "deploy")
	gitRun(t, root, "clone", remote, workdir)
	withGitCleanConfig(t, root, map[string]schema.GitRepository{
		"deploy": {URI: remote, Workdir: workdir},
	})

	err := runClean(context.Background(), &cleanOptions{DryRun: true}, []string{"deploy"})

	require.NoError(t, err)
	assert.DirExists(t, workdir)
}

func TestRunCleanExplicitWorkdirDeletesManagedClone(t *testing.T) {
	requireGit(t)

	root := t.TempDir()
	remote := initTestRemote(t, root)
	workdir := filepath.Join(root, "workdirs", "deploy")
	gitRun(t, root, "clone", remote, workdir)
	withGitCleanConfig(t, root, map[string]schema.GitRepository{
		"deploy": {URI: remote, Workdir: workdir},
	})

	err := runClean(context.Background(), &cleanOptions{Force: true}, []string{"deploy"})

	require.NoError(t, err)
	assert.NoDirExists(t, workdir)
}

func TestRunCleanNoForceDeletesManagedClone(t *testing.T) {
	requireGit(t)

	root := t.TempDir()
	remote := initTestRemote(t, root)
	workdir := filepath.Join(root, "workdirs", "deploy")
	gitRun(t, root, "clone", remote, workdir)
	withGitCleanConfig(t, root, map[string]schema.GitRepository{
		"deploy": {URI: remote, Workdir: workdir},
	})

	err := runClean(context.Background(), &cleanOptions{}, []string{"deploy"})

	require.NoError(t, err)
	assert.NoDirExists(t, workdir)
}

func TestRunCleanDirtyWorkdirRequiresForce(t *testing.T) {
	requireGit(t)

	root := t.TempDir()
	remote := initTestRemote(t, root)
	workdir := filepath.Join(root, "workdirs", "deploy")
	gitRun(t, root, "clone", remote, workdir)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "dirty.txt"), []byte("dirty\n"), 0o644))
	withGitCleanConfig(t, root, map[string]schema.GitRepository{
		"deploy": {URI: remote, Workdir: workdir},
	})

	err := runClean(context.Background(), &cleanOptions{}, []string{"deploy"})

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrRequiredFlagNotProvided))
	assert.DirExists(t, workdir)
}

func TestRunCleanDirtyWorkdirDeletesWithForce(t *testing.T) {
	requireGit(t)

	root := t.TempDir()
	remote := initTestRemote(t, root)
	workdir := filepath.Join(root, "workdirs", "deploy")
	gitRun(t, root, "clone", remote, workdir)
	require.NoError(t, os.WriteFile(filepath.Join(workdir, "dirty.txt"), []byte("dirty\n"), 0o644))
	withGitCleanConfig(t, root, map[string]schema.GitRepository{
		"deploy": {URI: remote, Workdir: workdir},
	})

	err := runClean(context.Background(), &cleanOptions{Force: true}, []string{"deploy"})

	require.NoError(t, err)
	assert.NoDirExists(t, workdir)
}

func TestRunCleanDefaultXDGWorkdirDeletesManagedClone(t *testing.T) {
	requireGit(t)

	root := t.TempDir()
	cacheRoot := filepath.Join(root, "cache")
	t.Setenv("ATMOS_XDG_CACHE_HOME", cacheRoot)
	remote := initTestRemote(t, root)
	workdir := atmosgit.DefaultWorkdirPath("deploy")
	require.NoError(t, os.MkdirAll(filepath.Dir(workdir), 0o755))
	gitRun(t, root, "clone", remote, workdir)
	withGitCleanConfig(t, root, map[string]schema.GitRepository{
		"deploy": {URI: remote},
	})

	err := runClean(context.Background(), &cleanOptions{Force: true}, []string{"deploy"})

	require.NoError(t, err)
	assert.NoDirExists(t, workdir)
}

func TestRunCleanAllDeletesConfiguredWorkdirs(t *testing.T) {
	requireGit(t)

	root := t.TempDir()
	remote := initTestRemote(t, root)
	deploy := filepath.Join(root, "workdirs", "deploy")
	infra := filepath.Join(root, "workdirs", "infra")
	gitRun(t, root, "clone", remote, deploy)
	gitRun(t, root, "clone", remote, infra)
	withGitCleanConfig(t, root, map[string]schema.GitRepository{
		"deploy": {URI: remote, Workdir: deploy},
		"infra":  {URI: remote, Workdir: infra},
	})

	err := runClean(context.Background(), &cleanOptions{All: true, Force: true}, nil)

	require.NoError(t, err)
	assert.NoDirExists(t, deploy)
	assert.NoDirExists(t, infra)
}

func TestRunCleanRefusesCurrentProjectWorkdir(t *testing.T) {
	root := t.TempDir()
	withGitCleanConfig(t, root, map[string]schema.GitRepository{
		"deploy": {URI: "git@github.com:cloudposse-sandbox/empty.git", Workdir: "."},
	})

	err := runClean(context.Background(), &cleanOptions{Force: true}, []string{"deploy"})

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig))
}

func TestRunCleanRefusesParentTraversalWorkdir(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	require.NoError(t, os.MkdirAll(project, 0o755))
	withGitCleanConfig(t, project, map[string]schema.GitRepository{
		"deploy": {URI: "git@github.com:cloudposse-sandbox/empty.git", Workdir: "./../../../"},
	})

	err := runClean(context.Background(), &cleanOptions{Force: true}, []string{"deploy"})

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig))
}

func TestRunCleanRefusesRemoteMismatch(t *testing.T) {
	requireGit(t)

	root := t.TempDir()
	remote := initTestRemote(t, filepath.Join(root, "one"))
	otherRemote := initTestRemote(t, filepath.Join(root, "two"))
	workdir := filepath.Join(root, "workdirs", "deploy")
	gitRun(t, root, "clone", remote, workdir)
	withGitCleanConfig(t, root, map[string]schema.GitRepository{
		"deploy": {URI: otherRemote, Workdir: workdir},
	})

	err := runClean(context.Background(), &cleanOptions{Force: true}, []string{"deploy"})

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig))
	assert.DirExists(t, workdir)
}

func TestRunCleanRefusesNonGitDirectory(t *testing.T) {
	root := t.TempDir()
	workdir := filepath.Join(root, "workdirs", "deploy")
	require.NoError(t, os.MkdirAll(workdir, 0o755))
	withGitCleanConfig(t, root, map[string]schema.GitRepository{
		"deploy": {URI: "git@github.com:cloudposse-sandbox/empty.git", Workdir: workdir},
	})

	err := runClean(context.Background(), &cleanOptions{Force: true}, []string{"deploy"})

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig))
	assert.DirExists(t, workdir)
}

func TestRunCleanRefusesDangerousXDGCacheHome(t *testing.T) {
	for name, cacheHome := range map[string]string{
		"root":     string(os.PathSeparator),
		"relative": "./../../../",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("ATMOS_XDG_CACHE_HOME", cacheHome)
			withGitCleanConfig(t, root, map[string]schema.GitRepository{
				"deploy": {URI: "git@github.com:cloudposse-sandbox/empty.git"},
			})

			err := runClean(context.Background(), &cleanOptions{Force: true}, []string{"deploy"})

			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig))
		})
	}
}

func TestRunCleanRefusesProjectXDGCacheHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", root)
	withGitCleanConfig(t, root, map[string]schema.GitRepository{
		"deploy": {URI: "git@github.com:cloudposse-sandbox/empty.git"},
	})

	err := runClean(context.Background(), &cleanOptions{Force: true}, []string{"deploy"})

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig))
}

func withGitCleanConfig(t *testing.T, basePath string, repositories map[string]schema.GitRepository) {
	t.Helper()

	originalConfig := atmosConfigPtr
	originalWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(basePath, 0o755))
	require.NoError(t, os.Chdir(basePath))

	atmosConfigPtr = &schema.AtmosConfiguration{
		BasePath: basePath,
		Git: schema.GitConfig{
			Repositories: repositories,
		},
	}

	t.Cleanup(func() {
		atmosConfigPtr = originalConfig
		require.NoError(t, os.Chdir(originalWD))
	})
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable is not available")
	}
}

func initTestRemote(t *testing.T, root string) string {
	t.Helper()

	seed := filepath.Join(root, "seed")
	remote := filepath.Join(root, "remote.git")
	require.NoError(t, os.MkdirAll(root, 0o755))
	gitRun(t, root, "init", "-b", "main", seed)
	gitRun(t, seed, "config", "user.name", "test")
	gitRun(t, seed, "config", "user.email", "test@example.com")
	require.NoError(t, os.WriteFile(filepath.Join(seed, "README.md"), []byte("test\n"), 0o644))
	gitRun(t, seed, "add", "README.md")
	gitRun(t, seed, "commit", "-m", "initial")
	gitRun(t, root, "clone", "--bare", seed, remote)
	return remote
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", gitTestArgs(args...)...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed:\n%s", args, string(out))
}
