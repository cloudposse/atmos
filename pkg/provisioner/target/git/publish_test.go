package git

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPublishRawFiles(t *testing.T) {
	isolatedGitEnv(t)
	root := t.TempDir()
	bare := seedBareRepo(t, root)
	workdir := filepath.Join(root, "workdir")
	source := filepath.Join(root, "source")
	require.NoError(t, os.Mkdir(source, 0o755))
	raw := []byte{'P', 'K', 0, 255, '\n'}
	require.NoError(t, os.WriteFile(filepath.Join(source, "binary.yaml"), raw, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(source, "keep.txt"), []byte("keep"), 0o600))
	files, err := target.LocalPublishFiles(source, "")
	require.NoError(t, err)
	env := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		env[key] = value
	}
	in := &target.PublishInput{
		AtmosConfig: &schema.AtmosConfiguration{Git: schema.GitConfig{Repositories: map[string]schema.GitRepository{
			"deployments": {URI: bare, Branch: "main", Workdir: workdir},
		}}},
		TargetConfig: map[string]any{"kind": "git", "repository": "deployments", "path": "output", "commit": map[string]any{"message": "Publish files", "signing": "never"}},
		Files:        files, Env: env,
	}
	g := &gitProvisioner{}
	result, err := g.Publish(t.Context(), in)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Changed)
	assert.NotEmpty(t, result.Metadata["commit"])
	first := gitCmd(t, bare, "rev-parse", "main")
	assert.Equal(t, raw, []byte(gitCmd(t, bare, "show", "main:output/binary.yaml")))
	require.NoError(t, os.Remove(filepath.Join(source, "keep.txt")))
	in.Files, err = target.LocalPublishFiles(source, "")
	require.NoError(t, err)
	result, err = g.Publish(t.Context(), in)
	require.NoError(t, err)
	assert.Zero(t, result.Changed)
	assert.Equal(t, 1, result.Unchanged)
	assert.Equal(t, first, gitCmd(t, bare, "rev-parse", "main"))
	assert.Equal(t, "keep", gitCmd(t, bare, "show", "main:output/keep.txt"))
	assert.Contains(t, gitCmd(t, bare, "show", "main:README.md"), "deployments")
	require.NoError(t, os.WriteFile(filepath.Join(source, "binary.yaml"), []byte("changed"), 0o600))
	in.Files, err = target.LocalPublishFiles(source, "")
	require.NoError(t, err)
	result, err = g.Publish(t.Context(), in)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Changed)
	assert.NotEqual(t, first, gitCmd(t, bare, "rev-parse", "main"))
}

func TestPublishRejectsDirectoryCollision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "artifact")
	require.NoError(t, os.WriteFile(source, []byte("binary"), 0o600))
	files, err := target.LocalPublishFiles(source, "")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "output", "artifact"), 0o755))
	_, err = writePublishFile(root, "output", files[0])
	require.Error(t, err)
	assert.DirExists(t, filepath.Join(root, "output", "artifact"))
}

func TestPublishRejectsUnsafeFileAndTargetPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_, err := writePublishFile(root, "output", target.PublishFile{Name: "../outside"})
	require.ErrorIs(t, err, errUtils.ErrPublishSource)
	_, err = writePublishFile(root, "../outside", target.PublishFile{Name: "artifact"})
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(filepath.Dir(root), "outside", "artifact"))
}

func TestPublishEmptyFilesDoesNotCreateCommit(t *testing.T) {
	isolatedGitEnv(t)
	root := t.TempDir()
	bare := seedBareRepo(t, root)
	initial := gitCmd(t, bare, "rev-parse", "main")
	in := &target.PublishInput{
		AtmosConfig: &schema.AtmosConfiguration{Git: schema.GitConfig{Repositories: map[string]schema.GitRepository{
			"deployments": {URI: bare, Branch: "main", Workdir: filepath.Join(root, "workdir")},
		}}},
		TargetConfig: map[string]any{"kind": "git", "repository": "deployments", "path": "output"},
	}
	result, err := (&gitProvisioner{}).Publish(t.Context(), in)
	require.NoError(t, err)
	assert.Empty(t, result.Locations)
	assert.Zero(t, result.Changed)
	assert.Equal(t, initial, gitCmd(t, bare, "rev-parse", "main"))
}

func TestPublishRejectsMalformedGitSettings(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		field string
		value any
	}{
		{"repository type", "repository", 42},
		{"path type", "path", true},
		{"commit type", "commit", "message"},
		{"message type", "commit", map[string]any{"message": true}},
		{"signing type", "commit", map[string]any{"signing": true}},
		{"signing mode", "commit", map[string]any{"signing": "sometimes"}},
		{"unknown commit field", "commit", map[string]any{"unknown": "value"}},
		{"pull request type", "pull_request", false},
		{"pull request enabled type", "pull_request", map[string]any{"enabled": "false"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			settings := map[string]any{"kind": "git", "repository": "deployments", "path": "output"}
			settings[tc.field] = tc.value
			err := (&gitProvisioner{}).ValidatePublish(&target.PublishInput{TargetConfig: settings})
			require.ErrorIs(t, err, errUtils.ErrPublishTarget)
		})
	}
}

func TestPublishRetriesFailedPushWithUnchangedFiles(t *testing.T) {
	isolatedGitEnv(t)
	root := t.TempDir()
	bare := seedBareRepo(t, root)
	workdir := filepath.Join(root, "workdir")
	source := filepath.Join(root, "artifact")
	require.NoError(t, os.WriteFile(source, []byte("artifact bytes"), 0o600))
	files, err := target.LocalPublishFiles(source, "")
	require.NoError(t, err)
	env := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		env[key] = value
	}
	in := &target.PublishInput{
		AtmosConfig: &schema.AtmosConfiguration{Git: schema.GitConfig{Repositories: map[string]schema.GitRepository{
			"deployments": {URI: bare, Branch: "main", Workdir: workdir},
		}}},
		TargetConfig: map[string]any{"kind": "git", "repository": "deployments", "path": "output", "commit": map[string]any{"message": "Publish files", "signing": "never"}},
		Files:        files, Env: env,
	}
	first := gitCmd(t, bare, "rev-parse", "main")
	gitCmd(t, root, "clone", "--branch", "main", bare, workdir)
	unavailableRemote := filepath.Join(root, "unavailable.git")
	gitCmd(t, workdir, "remote", "set-url", "--push", "origin", unavailableRemote)
	_, err = (&gitProvisioner{}).Publish(t.Context(), in)
	require.Error(t, err)
	assert.Equal(t, first, gitCmd(t, bare, "rev-parse", "main"))
	localCommit := gitCmd(t, workdir, "rev-parse", "HEAD")
	assert.NotEqual(t, first, localCommit)
	gitCmd(t, workdir, "remote", "set-url", "--push", "origin", bare)
	result, err := (&gitProvisioner{}).Publish(t.Context(), in)
	require.NoError(t, err)
	assert.Zero(t, result.Changed)
	assert.Equal(t, 1, result.Unchanged)
	assert.Equal(t, localCommit, gitCmd(t, bare, "rev-parse", "main"))
	assert.Equal(t, "artifact bytes", gitCmd(t, bare, "show", "main:output/artifact"))
	// A synchronized unchanged publish must not attempt another push.
	gitCmd(t, workdir, "remote", "set-url", "--push", "origin", unavailableRemote)
	result, err = (&gitProvisioner{}).Publish(t.Context(), in)
	require.NoError(t, err)
	assert.Zero(t, result.Changed)
	assert.Equal(t, 1, result.Unchanged)
}

func TestPublishDefaultBranch(t *testing.T) {
	isolatedGitEnv(t)
	root := t.TempDir()
	bare := seedBareRepo(t, root)
	workdir := filepath.Join(root, "workdir")
	source := filepath.Join(root, "artifact")
	require.NoError(t, os.WriteFile(source, []byte("default branch artifact"), 0o600))
	files, err := target.LocalPublishFiles(source, "")
	require.NoError(t, err)
	env := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		env[key] = value
	}
	in := &target.PublishInput{
		AtmosConfig: &schema.AtmosConfiguration{Git: schema.GitConfig{Repositories: map[string]schema.GitRepository{
			"deployments": {URI: bare, Workdir: workdir},
		}}},
		TargetConfig: map[string]any{"kind": "git", "repository": "deployments", "path": "output", "commit": map[string]any{"message": "Publish default branch", "signing": "never"}},
		Files:        files, Env: env,
	}
	result, err := (&gitProvisioner{}).Publish(t.Context(), in)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Changed)
	assert.NotEmpty(t, result.Metadata["commit"])
	assert.NoFileExists(t, filepath.Join(workdir, ".git", "FETCH_HEAD"), "a fresh clone does not need FETCH_HEAD to detect unpublished commits")
	assert.Equal(t, "default branch artifact", gitCmd(t, bare, "show", "main:output/artifact"))
	gitCmd(t, workdir, "remote", "set-url", "--push", "origin", filepath.Join(root, "unavailable.git"))
	result, err = (&gitProvisioner{}).Publish(t.Context(), in)
	require.NoError(t, err)
	assert.Zero(t, result.Changed)
	assert.Equal(t, 1, result.Unchanged)
}

func TestPublishRejectsUnsafeGitTargets(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		field string
		value any
		want  error
	}{
		{"missing repository", "repository", "", errUtils.ErrPublishTarget},
		{"root path", "path", ".", errUtils.ErrPublishTarget},
		{"parent path", "path", "../outside", errUtils.ErrPublishSource},
		{"git metadata", "path", ".git/objects", errUtils.ErrPublishSource},
		{"split", "split", false, errUtils.ErrPublishTarget},
		{"invalid split", "split", "false", errUtils.ErrGitTargetSplitInvalid},
		{"pull request", "pull_request", map[string]any{"enabled": true}, errUtils.ErrPublishTarget},
		{"unknown pull request option", "pull_request", map[string]any{"title": "publish"}, errUtils.ErrPublishTarget},
		{"unknown option", "delete", true, errUtils.ErrPublishTarget},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			settings := map[string]any{"kind": "git", "repository": "deployments", "path": "output"}
			settings[tc.field] = tc.value
			result, err := (&gitProvisioner{}).Publish(t.Context(), &target.PublishInput{TargetConfig: settings})
			require.ErrorIs(t, err, tc.want)
			assert.Nil(t, result)
		})
	}
}

func TestPublishIdentityFailureLeavesRepositoryUntouched(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	workdir := filepath.Join(root, "workdir")
	in := &target.PublishInput{
		AtmosConfig: &schema.AtmosConfiguration{Git: schema.GitConfig{Repositories: map[string]schema.GitRepository{
			"deployments": {URI: filepath.Join(root, "remote.git"), Branch: "main", Workdir: workdir},
		}}},
		TargetConfig: map[string]any{"kind": "git", "repository": "deployments", "path": "output", "auth": map[string]any{"identity": "publisher"}, "pull_request": map[string]any{"enabled": false}},
		EnvProvider:  &errEnvProvider{err: context.Canceled},
		Env:          map[string]string{"GIT_TERMINAL_PROMPT": "0"},
	}
	result, err := (&gitProvisioner{}).Publish(t.Context(), in)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)
	assert.NoDirExists(t, workdir)
}

func TestPublishRejectsSymlinkDestination(t *testing.T) {
	t.Parallel()
	root, outside := t.TempDir(), t.TempDir()
	original := filepath.Join(outside, "artifact")
	require.NoError(t, os.WriteFile(original, []byte("preserve me"), 0o600))
	if err := os.Symlink(outside, filepath.Join(root, "output")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation requires Windows privileges: %v", err)
		}
		require.NoError(t, err)
	}
	files, err := target.LocalPublishFiles(original, "")
	require.NoError(t, err)
	changed, err := writePublishFile(root, "output", files[0])
	require.Error(t, err)
	assert.False(t, changed)
	content, err := os.ReadFile(original)
	require.NoError(t, err)
	assert.Equal(t, "preserve me", string(content))
}

func TestPublishCanceledOrMissingSourcePreservesDestination(t *testing.T) {
	t.Parallel()
	for _, canceled := range []bool{false, true} {
		name := "missing source"
		if canceled {
			name = "canceled"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			source := filepath.Join(t.TempDir(), "artifact")
			require.NoError(t, os.WriteFile(source, []byte("new content"), 0o600))
			files, err := target.LocalPublishFiles(source, "")
			require.NoError(t, err)
			require.NoError(t, os.Mkdir(filepath.Join(root, "output"), 0o755))
			dest := filepath.Join(root, "output", "artifact")
			require.NoError(t, os.WriteFile(dest, []byte("original content"), 0o600))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			want := os.ErrNotExist
			if canceled {
				cancel()
				want = context.Canceled
			} else {
				require.NoError(t, os.Remove(source))
			}
			result := &target.PublishResult{}
			err = writePublishFiles(ctx, &repoSession{rc: atmosgit.RepoContext{Workdir: root}}, "output", files, result)
			require.ErrorIs(t, err, want)
			assert.Empty(t, result.Locations)
			assert.Zero(t, result.Changed)
			content, err := os.ReadFile(dest)
			require.NoError(t, err)
			assert.Equal(t, "original content", string(content))
		})
	}
}
