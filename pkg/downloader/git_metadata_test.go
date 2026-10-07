package downloader

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFetchWithMetadataGitSubdirectory(t *testing.T) {
	requireRealGit(t)
	repoDir := initCloneTestGitRepo(t, map[string]string{"example/file.txt": "v1"})
	wantCommit := runCloneTestGit(t, repoDir, "rev-parse", "HEAD")
	dest := filepath.Join(t.TempDir(), "download")
	download := NewGoGetterDownloader(&schema.AtmosConfiguration{})
	src := "git::" + cloneTestGitFileURI(repoDir) + "//example?ref=main&depth=1"
	metadata, err := download.FetchWithMetadata(src, dest, ClientModeDir, time.Minute)
	require.NoError(t, err)
	assert.Equal(t, wantCommit, metadata.GitCommit)
	assert.NoDirExists(t, filepath.Join(dest, ".git"))
	content, err := os.ReadFile(filepath.Join(dest, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v1", string(content))

	// Reusing the downloader must capture each checkout's own identity.
	writeAndCommit(t, repoDir, "example/file.txt", "v2")
	wantCommit = runCloneTestGit(t, repoDir, "rev-parse", "HEAD")
	metadata, err = download.FetchWithMetadata(src, filepath.Join(t.TempDir(), "next"), ClientModeDir, time.Minute)
	require.NoError(t, err)
	assert.Equal(t, wantCommit, metadata.GitCommit)

	// A later failure must not return stale provenance from the previous download.
	metadata, err = download.FetchWithMetadata("git::"+cloneTestGitFileURI(repoDir)+"//example?ref=missing", filepath.Join(t.TempDir(), "failed"), ClientModeDir, time.Minute)
	require.Error(t, err)
	assert.Empty(t, metadata.GitCommit)
}

func TestCustomGitGetterClearsCommitAfterFailure(t *testing.T) {
	g := &CustomGitGetter{ResolvedCommit: "previous"}
	t.Setenv("PATH", "")
	src, err := url.Parse("https://example.com/repo.git")
	require.NoError(t, err)
	require.Error(t, g.Get(t.TempDir(), src))
	assert.Empty(t, g.ResolvedCommit)
}
