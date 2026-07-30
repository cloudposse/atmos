package source

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/schema"
)

const sampleScaffold = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: sample
spec:
  fields:
    - name: project_name
      type: input
      default: demo
`

// writeSampleTemplate creates a minimal on-disk scaffold template and returns its directory.
func writeSampleTemplate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scaffold.yaml"), []byte(sampleScaffold), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o600))
	return dir
}

func TestIsTemplateSource(t *testing.T) {
	assert.True(t, IsTemplateSource("github.com/acme/template"))
	assert.True(t, IsTemplateSource("git::https://example.com/acme/template.git"))
	assert.True(t, IsTemplateSource("./local-template"))
	assert.True(t, IsTemplateSource("/tmp/local-template"))
	assert.False(t, IsTemplateSource("aws/landing-zone"))
	assert.False(t, IsTemplateSource("basic"))
}

func TestWithRef(t *testing.T) {
	assert.Equal(t, "github.com/acme/template?ref=v1.2.3", WithRef("github.com/acme/template", "v1.2.3"))
	assert.Equal(t, "github.com/acme/template//scaffold?ref=v1.2.3", WithRef("github.com/acme/template//scaffold", "v1.2.3"))
	assert.Equal(t, "github.com/acme/template?depth=1&ref=v1.2.3", WithRef("github.com/acme/template?depth=1", "v1.2.3"))
	assert.Equal(t, "github.com/acme/template?ref=main", WithRef("github.com/acme/template?ref=main", "v1.2.3"))
	assert.Equal(t, "./local", WithRef("./local", "v1.2.3"))
}

func hasSampleFile(files []templates.File) bool {
	for _, f := range files {
		if f.Path == "file.txt" {
			return true
		}
	}
	return false
}

func TestResolve_LocalPath(t *testing.T) {
	dir := writeSampleTemplate(t)

	cfg, cleanup, err := Resolve(&schema.AtmosConfiguration{}, "sample", dir, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()
	require.NotNil(t, cfg)
	assert.True(t, hasSampleFile(cfg.Files), "local template files must be loaded")
}

func TestResolve_LocalPathDefaultTimeout(t *testing.T) {
	dir := writeSampleTemplate(t)

	cfg, cleanup, err := Resolve(&schema.AtmosConfiguration{}, "sample", dir, 0)
	require.NoError(t, err)
	defer cleanup()
	require.NotNil(t, cfg)
	assert.True(t, hasSampleFile(cfg.Files))
}

func TestResolve_FileURI(t *testing.T) {
	dir := writeSampleTemplate(t)

	cfg, cleanup, err := Resolve(&schema.AtmosConfiguration{}, "sample", "file://"+dir, time.Minute)
	require.NoError(t, err)
	defer cleanup()
	require.NotNil(t, cfg)
	assert.True(t, hasSampleFile(cfg.Files))
}

func TestResolve_BadLocalPathReturnsLoadError(t *testing.T) {
	_, cleanup, err := Resolve(&schema.AtmosConfiguration{}, "missing", filepath.Join(t.TempDir(), "missing"), time.Minute)
	require.Error(t, err)
	require.NotNil(t, cleanup)
	cleanup()
}

func TestResolve_LocalPathMissingScaffoldConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("not a scaffold"), 0o600))

	_, cleanup, err := Resolve(&schema.AtmosConfiguration{}, "missing-scaffold", dir, time.Minute)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldConfigMissing)
	require.NotNil(t, cleanup)
	cleanup()
}

func TestResolve_OCIUnsupported(t *testing.T) {
	_, cleanup, err := Resolve(&schema.AtmosConfiguration{}, "x", "oci://ghcr.io/cloudposse/x:latest", time.Minute)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldSourceUnsupported)
	require.NotNil(t, cleanup)
	cleanup()
}

func TestResolve_RemoteFetchFailureCleansUp(t *testing.T) {
	_, cleanup, err := Resolve(&schema.AtmosConfiguration{}, "x", "git::file:///definitely/not/a/repo", time.Millisecond)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldFetchSource)
	require.NotNil(t, cleanup)
	cleanup()
}

func TestHydrate_NoopForFullConfig(t *testing.T) {
	stub := &templates.Configuration{Name: "x", Files: []templates.File{{Path: "a"}}}
	cleanup, err := Hydrate(stub, "")
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	cleanup()
	assert.Len(t, stub.Files, 1, "full configs are returned unchanged")
}

func TestHydrate_NoopForEmptySource(t *testing.T) {
	stub := &templates.Configuration{Name: "x"}
	cleanup, err := Hydrate(stub, "")
	require.NoError(t, err)
	cleanup()
	assert.Empty(t, stub.Files)
}

func TestHydrate_LocalStub(t *testing.T) {
	dir := writeSampleTemplate(t)

	stub := &templates.Configuration{Name: "sample", Source: dir}
	cleanup, err := Hydrate(stub, "")
	require.NoError(t, err)
	defer cleanup()
	assert.True(t, hasSampleFile(stub.Files), "local stub must be hydrated from its source")
}

func TestHydrate_LocalStubError(t *testing.T) {
	stub := &templates.Configuration{Name: "missing", Source: filepath.Join(t.TempDir(), "missing")}

	cleanup, err := Hydrate(stub, "")
	require.Error(t, err)
	require.NotNil(t, cleanup)
	cleanup()
}

// requireGit skips the test when the git binary is unavailable, matching the
// inline-skip convention used elsewhere in the codebase for git-backed tests.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not found on PATH")
	}
}

// initSourceTestGitRepo creates a local git repository on branch "main" with the given
// files committed, mirroring the local-git-fixture pattern used by
// tests/cli_remote_imports_test.go and pkg/stack/imports/remote_test.go.
func initSourceTestGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()

	repoDir := t.TempDir()
	runSourceTestGit(t, repoDir, "init")
	runSourceTestGit(t, repoDir, "checkout", "-b", "main")
	runSourceTestGit(t, repoDir, "config", "user.email", "test@example.com")
	runSourceTestGit(t, repoDir, "config", "user.name", "Test User")

	for name, content := range files {
		path := filepath.Join(repoDir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	runSourceTestGit(t, repoDir, "add", ".")
	runSourceTestGit(t, repoDir, "commit", "-m", "initial")
	return repoDir
}

func runSourceTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
}

func sourceTestGitFileURI(path string) string {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if filepath.VolumeName(path) != "" && cleaned != "" && cleaned[0] != '/' {
		cleaned = "/" + cleaned
	}
	return (&url.URL{Scheme: "file", Path: cleaned}).String()
}

// TestResolve_RemoteGitSubdirSuccess exercises the remote-fetch branch of Resolve
// (resolver.go's go-getter path) against a real local git remote using go-getter's
// //subdir?ref= syntax — the exact mechanism `atmos init aws/app` relies on. No prior
// test drove a successful fetch through this path; every other scaffold test sets
// ATMOS_SCAFFOLD_SOURCE_OVERRIDE and bypasses it entirely.
func TestResolve_RemoteGitSubdirSuccess(t *testing.T) {
	requireGit(t)

	repoDir := initSourceTestGitRepo(t, map[string]string{
		"aws/app/scaffold.yaml": sampleScaffold,
		"aws/app/file.txt":      "hello",
	})
	src := "git::" + sourceTestGitFileURI(repoDir) + "//aws/app?ref=main"

	cfg, cleanup, err := Resolve(&schema.AtmosConfiguration{}, "aws/app", src, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, cleanup)
	defer cleanup()
	require.NotNil(t, cfg)
	assert.True(t, hasSampleFile(cfg.Files), "remote git subdir template files must be loaded")
}

// TestResolve_RemoteGitSubdirMissing pins the exact failure mode reported for
// `atmos init aws/app`: a valid git remote whose requested //subdir does not exist.
func TestResolve_RemoteGitSubdirMissing(t *testing.T) {
	requireGit(t)

	repoDir := initSourceTestGitRepo(t, map[string]string{
		"aws/app/scaffold.yaml": sampleScaffold,
		"aws/app/file.txt":      "hello",
	})
	src := "git::" + sourceTestGitFileURI(repoDir) + "//aws/missing?ref=main"

	_, cleanup, err := Resolve(&schema.AtmosConfiguration{}, "aws/missing", src, time.Minute)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrScaffoldFetchSource)
	require.NotNil(t, cleanup)
	cleanup()
}
