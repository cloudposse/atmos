package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestParseConfig(t *testing.T) {
	block := map[string]any{
		"repository": "deployments",
		"path":       "clusters/dev/argocd",
		"auth":       map[string]any{"identity": "platform-admin"},
		"commit": map[string]any{
			"message": "Render argocd",
			"signing": "always",
		},
		"pull_request": map[string]any{"enabled": true},
	}
	cfg := parseConfig(block)
	assert.Equal(t, "deployments", cfg.Repository)
	assert.Equal(t, "clusters/dev/argocd", cfg.Path)
	assert.Equal(t, "platform-admin", cfg.Identity)
	assert.Equal(t, "Render argocd", cfg.CommitMessage)
	assert.Equal(t, "always", cfg.Signing)
	assert.True(t, cfg.PullRequest)
}

func TestParseConfigEmpty(t *testing.T) {
	cfg := parseConfig(map[string]any{})
	assert.Empty(t, cfg.Repository)
	assert.Empty(t, cfg.Identity)
	assert.False(t, cfg.PullRequest)
}

func TestDeliverPullRequestNotSupported(t *testing.T) {
	g := &gitProvisioner{}
	err := g.Deliver(context.Background(), &target.DeliverInput{
		AtmosConfig:  &schema.AtmosConfiguration{},
		TargetName:   "deployment-repo",
		TargetConfig: map[string]any{"repository": "deployments", "pull_request": map[string]any{"enabled": true}},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGitPullRequestNotSupported)
}

func TestDeliverRepositoryNotFound(t *testing.T) {
	g := &gitProvisioner{}
	err := g.Deliver(context.Background(), &target.DeliverInput{
		AtmosConfig:  &schema.AtmosConfiguration{}, // no git.repositories configured.
		TargetName:   "deployment-repo",
		TargetConfig: map[string]any{"repository": "missing"},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGitRepositoryNotFound)
}

func TestWriteArtifactReplacesManagedSubtree(t *testing.T) {
	workdir := t.TempDir()
	path := "clusters/dev/argocd"

	// First write: two files plus a stale file that should be removed on rewrite.
	first := target.ProvisionArtifact{Files: map[string][]byte{
		"namespace.yaml":  []byte("kind: Namespace\n"),
		"deployment.yaml": []byte("kind: Deployment\n"),
		"old/stale.yaml":  []byte("stale\n"),
	}}
	require.NoError(t, writeArtifact(workdir, path, &first))
	assert.FileExists(t, filepath.Join(workdir, "clusters", "dev", "argocd", "namespace.yaml"))
	assert.FileExists(t, filepath.Join(workdir, "clusters", "dev", "argocd", "old", "stale.yaml"))

	// Second write without the stale file: managed subtree is replaced, stale file gone.
	second := target.ProvisionArtifact{Files: map[string][]byte{
		"namespace.yaml": []byte("kind: Namespace\n"),
	}}
	require.NoError(t, writeArtifact(workdir, path, &second))
	assert.FileExists(t, filepath.Join(workdir, "clusters", "dev", "argocd", "namespace.yaml"))
	assert.NoFileExists(t, filepath.Join(workdir, "clusters", "dev", "argocd", "old", "stale.yaml"))
	assert.NoFileExists(t, filepath.Join(workdir, "clusters", "dev", "argocd", "deployment.yaml"))
}

func TestWriteArtifactRejectsPathEscape(t *testing.T) {
	workdir := t.TempDir()
	err := writeArtifact(workdir, "../escape", &target.ProvisionArtifact{Files: map[string][]byte{"x.yaml": []byte("x")}})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrGitPathEscapesWorktree)
}

// isolatedGitEnv configures the process env so the developer's gitconfig cannot
// leak in and "main" is the deterministic default branch.
func isolatedGitEnv(t *testing.T) {
	t.Helper()
	globalCfg := filepath.Join(t.TempDir(), "gitconfig")
	cfg := "[init]\n\tdefaultBranch = main\n[user]\n\tname = Test User\n\temail = test@example.com\n"
	require.NoError(t, os.WriteFile(globalCfg, []byte(cfg), 0o600))
	t.Setenv("GIT_CONFIG_GLOBAL", globalCfg)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
}

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	result, err := atmosgit.NewExecRunner().Run(context.Background(), "git", args, atmosgit.RunOptions{Dir: dir, Env: os.Environ()})
	require.NoError(t, err, "git %v: %s", args, result.StderrTail)
	return result.Stdout
}

// seedBareRepo creates a bare repo with an initial commit on main and returns its path.
func seedBareRepo(t *testing.T, root string) string {
	t.Helper()
	bare := filepath.Join(root, "origin.git")
	gitCmd(t, "", "init", "--bare", bare)
	gitCmd(t, bare, "symbolic-ref", "HEAD", "refs/heads/main")

	seed := filepath.Join(root, "seed")
	gitCmd(t, "", "clone", bare, seed)
	require.NoError(t, os.WriteFile(filepath.Join(seed, "README.md"), []byte("# deployments\n"), 0o600))
	gitCmd(t, seed, "add", "-A")
	gitCmd(t, seed, "commit", "-m", "init")
	gitCmd(t, seed, "push", "origin", "main")
	return bare
}

func TestDeliverIntegrationPublishesToRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	isolatedGitEnv(t)

	root := t.TempDir()
	bare := seedBareRepo(t, root)

	atmosConfig := &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"deployments": {
					URI:     bare,
					Branch:  "main",
					Workdir: filepath.Join(root, "workdir"),
				},
			},
		},
	}
	in := &target.DeliverInput{
		AtmosConfig: atmosConfig,
		TargetName:  "deployment-repo",
		TargetConfig: map[string]any{
			"repository": "deployments",
			"path":       "clusters/dev/argocd",
			"commit":     map[string]any{"message": "Render argocd for dev", "signing": "never"},
		},
		Artifact: target.ProvisionArtifact{
			Kind:   target.ArtifactKindKubernetesManifests,
			Format: target.FormatYAML,
			Files: map[string][]byte{
				"namespace.yaml":  []byte("apiVersion: v1\nkind: Namespace\n"),
				"deployment.yaml": []byte("apiVersion: apps/v1\nkind: Deployment\n"),
			},
			Metadata: target.ArtifactMetadata{Stack: "dev", Component: "argocd"},
		},
	}

	g := &gitProvisioner{}
	require.NoError(t, g.Deliver(context.Background(), in))

	// Verify by cloning the bare repo fresh.
	verify := filepath.Join(root, "verify")
	gitCmd(t, "", "clone", bare, verify)
	assert.FileExists(t, filepath.Join(verify, "clusters", "dev", "argocd", "namespace.yaml"))
	assert.FileExists(t, filepath.Join(verify, "clusters", "dev", "argocd", "deployment.yaml"))
	body := gitCmd(t, verify, "log", "-1", "--format=%B")
	assert.Contains(t, body, "Render argocd for dev")
	assert.Contains(t, body, "Atmos-Stack: dev")
	assert.Contains(t, body, "Atmos-Component: argocd")

	commitsBefore := gitCmd(t, verify, "rev-list", "--count", "HEAD")

	// Re-deliver the same artifact: a no-op, no new commit.
	require.NoError(t, g.Deliver(context.Background(), in))
	gitCmd(t, verify, "pull", "--ff-only")
	commitsAfter := gitCmd(t, verify, "rev-list", "--count", "HEAD")
	assert.Equal(t, commitsBefore, commitsAfter, "re-delivering identical artifact must be a no-op")
}
