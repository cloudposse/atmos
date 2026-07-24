package downloader

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/oci/ocitest"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

func TestResolveArtifactUsesContentHashForLocalDirectory(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "main.tf"), []byte("terraform {}"), 0o644))
	artifact, err := ResolveArtifact(context.Background(), nil, directory, directory)
	require.NoError(t, err)
	require.Equal(t, "local", artifact.Kind)
	require.Regexp(t, `^sha256:[a-f0-9]{64}$`, artifact.Identity)
}

func TestRedactSourceRemovesCredentialsAndQuery(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "empty string stays empty", source: "", want: ""},
		{name: "userinfo and query are stripped from a real URL", source: "https://token@example@github.example.com/org/module.git?signature=secret", want: "https://github.example.com/org/module.git"},
		{name: "non-URL text with a query-like suffix falls back to prefix split", source: "not a url?query=1", want: "not a url"},
		{name: "malformed percent-encoding falls back to prefix split", source: "%zz?foo=bar", want: "%zz"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, RedactSource(test.source))
		})
	}
}

// TestResolveArtifactOCISuccess resolves an "oci://" declared source against a
// real (in-process) registry and asserts the artifact is identified by the
// registry's manifest digest, not the mutable tag.
func TestResolveArtifactOCISuccess(t *testing.T) {
	imageRef := ocitest.NewRegistry(t, "resolve-artifact/success:v1", map[string]string{
		"main.tf": "# artifact\n",
	})

	artifact, err := ResolveArtifact(context.Background(), &schema.AtmosConfiguration{}, "oci://"+imageRef, "")
	require.NoError(t, err)
	assert.Equal(t, "oci", artifact.Kind)
	assert.Equal(t, imageRef, artifact.Resolved)
	assert.Regexp(t, `^sha256:[a-f0-9]{64}$`, artifact.Identity)
}

// TestResolveArtifactOCIError asserts a malformed "oci://" reference surfaces
// the underlying resolution error (no network call needed: name.ParseReference
// rejects it before any registry round trip) and leaves the artifact at its
// pre-resolution defaults.
func TestResolveArtifactOCIError(t *testing.T) {
	artifact, err := ResolveArtifact(context.Background(), &schema.AtmosConfiguration{}, "oci://invalid::image//name", "")
	require.Error(t, err)
	assert.Equal(t, "archive", artifact.Kind)
	assert.Empty(t, artifact.Identity)
}

// TestResolveArtifactDigestReference asserts a declared source that already
// embeds a "@sha256:" digest is trusted as the identity directly, without
// touching the filesystem or staging path.
func TestResolveArtifactDigestReference(t *testing.T) {
	digest := "sha256:" + strings.Repeat("ab", 32)
	declared := "https://example.com/archive.tar.gz@" + digest

	artifact, err := ResolveArtifact(context.Background(), nil, declared, "")
	require.NoError(t, err)
	assert.Equal(t, "archive", artifact.Kind)
	assert.Equal(t, digest, artifact.Identity)
}

// TestResolveArtifactGitStagedPath asserts that when the staging directory is
// a real Git checkout, the artifact identity is the checkout's HEAD commit
// rather than a content hash -- Git provenance is the stronger identity.
func TestResolveArtifactGitStagedPath(t *testing.T) {
	repoDir, headCommit := initGitRepoWithCommit(t)

	artifact, err := ResolveArtifact(context.Background(), nil, "https://github.example.com/org/module.git", repoDir)
	require.NoError(t, err)
	assert.Equal(t, "git", artifact.Kind)
	assert.Equal(t, headCommit, artifact.Identity)
}

// TestResolveArtifactStagedPathMissingFallsBackToError asserts that when the
// staging path exists nowhere -- neither a Git checkout nor a readable
// directory -- ResolveArtifact surfaces the underlying TreeSHA256 walk error
// rather than silently returning an empty identity.
func TestResolveArtifactStagedPathMissingFallsBackToError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := ResolveArtifact(context.Background(), nil, "https://example.com/archive.tar.gz", missing)
	require.Error(t, err)
}

// TestResolveArtifactDeclaredDirectoryWithoutStagedPath asserts the
// declared-path-is-a-local-directory fallback: when no staging path is given
// but the declared source itself is an existing directory, ResolveArtifact
// hashes that directory directly.
func TestResolveArtifactDeclaredDirectoryWithoutStagedPath(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "main.tf"), []byte("terraform {}"), 0o644))

	artifact, err := ResolveArtifact(context.Background(), nil, directory, "")
	require.NoError(t, err)
	assert.Equal(t, "local", artifact.Kind)
	assert.Regexp(t, `^sha256:[a-f0-9]{64}$`, artifact.Identity)
}

// TestDigestFromReference covers both branches directly: a reference carrying
// an "@sha256:" digest suffix, and one without.
func TestDigestFromReference(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "with digest suffix",
			source: "oci://ghcr.io/org/image@sha256:" + strings.Repeat("cd", 32),
			want:   "sha256:" + strings.Repeat("cd", 32),
		},
		{
			name:   "without digest suffix",
			source: "oci://ghcr.io/org/image:v1",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, digestFromReference(tt.source))
		})
	}
}

// TestGitCommitReturnsHead asserts gitCommit shells out to the real git
// binary and returns the checkout's trimmed HEAD commit SHA.
func TestGitCommitReturnsHead(t *testing.T) {
	repoDir, headCommit := initGitRepoWithCommit(t)

	commit, err := gitCommit(repoDir)
	require.NoError(t, err)
	assert.Equal(t, headCommit, commit)
}

// TestGitCommitErrorsOnNonGitDirectory asserts gitCommit surfaces the native
// git error (rather than a zero-value success) when directory isn't a Git
// checkout at all.
func TestGitCommitErrorsOnNonGitDirectory(t *testing.T) {
	tests.RequireExecutable(t, "git", "gitCommit non-repo test")

	_, err := gitCommit(t.TempDir())
	require.Error(t, err)
}

// TestTreeSHA256NonexistentRoot asserts a missing root directory is reported
// as a wrapped walk error, not a zero-value/empty hash.
func TestTreeSHA256NonexistentRoot(t *testing.T) {
	_, err := TreeSHA256(filepath.Join(t.TempDir(), "does-not-exist"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "walk artifact")
}

// TestTreeSHA256SkipsGitSubdirectoryButIncludesOthers is a behavioral test for
// the ".git" exclusion: two trees that differ only inside ".git" must hash
// identically, while a change to a tracked file (including one nested in a
// regular, non-".git" subdirectory) must change the hash.
func TestTreeSHA256SkipsGitSubdirectoryButIncludesOthers(t *testing.T) {
	buildTree := func(t *testing.T, trackedContent, gitContent string) string {
		t.Helper()
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "main.tf"), []byte(trackedContent), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte(gitContent), 0o644))
		return root
	}

	baseline := buildTree(t, "tracked-v1", "ref: refs/heads/main\n")
	sameTrackedDifferentGit := buildTree(t, "tracked-v1", "ref: refs/heads/other\n")
	differentTracked := buildTree(t, "tracked-v2", "ref: refs/heads/main\n")

	hashBaseline, err := TreeSHA256(baseline)
	require.NoError(t, err)
	hashSameTracked, err := TreeSHA256(sameTrackedDifferentGit)
	require.NoError(t, err)
	hashDifferentTracked, err := TreeSHA256(differentTracked)
	require.NoError(t, err)

	assert.Equal(t, hashBaseline, hashSameTracked, ".git contents must not affect the tree identity")
	assert.NotEqual(t, hashBaseline, hashDifferentTracked, "a tracked file change must change the tree identity")
}

// TestTreeSHA256IncludesSymlinkTarget is a behavioral test asserting that
// symlinks contribute their link target (not dereferenced content) to the
// hash: two trees whose only difference is what a same-named symlink points
// to must hash differently.
func TestTreeSHA256IncludesSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}

	buildTreeWithSymlink := func(t *testing.T, target string) string {
		t.Helper()
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "real.txt"), []byte("real"), 0o644))
		require.NoError(t, os.Symlink(target, filepath.Join(root, "link")))
		return root
	}

	hashA, err := TreeSHA256(buildTreeWithSymlink(t, "./real.txt"))
	require.NoError(t, err)
	hashB, err := TreeSHA256(buildTreeWithSymlink(t, "./other-target.txt"))
	require.NoError(t, err)

	assert.NotEqual(t, hashA, hashB, "differing symlink targets must produce differing tree identities")
}

// initGitRepoWithCommit creates a real Git repository (native git, not a
// stub) with a single commit and returns its directory and HEAD commit SHA,
// verified independently via `git rev-parse HEAD` so the test doesn't assume
// gitCommit's own correctness while testing it.
func initGitRepoWithCommit(t *testing.T) (string, string) {
	t.Helper()

	tests.RequireExecutable(t, "git", "gitCommit tests")

	repoDir := t.TempDir()
	runGit(t, repoDir, "init")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# test\n"), 0o644))
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir,
		"-c", "user.name=Atmos Test",
		"-c", "user.email=atmos@example.com",
		"-c", "commit.gpgsign=false",
		"commit", "-m", "initial")

	head := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "HEAD"))
	return repoDir, head
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(output))
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(output))
	return string(output)
}
