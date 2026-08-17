package helm

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/repo/v1"

	"github.com/cloudposse/atmos/pkg/schema"
)

// skipIfCannotDenyDirWrite skips tests that rely on removing write permission
// from a directory (or the permission bits on a pre-created lock file) to force
// a failure: the trick is a no-op on Windows (permissions work differently) and
// on Unix when running as root (root bypasses permission checks).
func skipIfCannotDenyDirWrite(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("directory write-permission bits are not enforced the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}
}

func TestMergeRepositories(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Components.Helm.Repositories = []schema.HelmRepository{
		{Name: "global", URL: "https://global.example.com"},
		{Name: "shared", URL: "https://old.example.com"},
	}
	section := map[string]any{
		"repositories": []any{
			map[string]any{
				"name":                     "shared",
				"url":                      "https://new.example.com",
				"username":                 "user",
				"password":                 "pass",
				"pass_credentials_all":     true,
				"cert_file":                "cert.pem",
				"key_file":                 "key.pem",
				"ca_file":                  "ca.pem",
				"insecure_skip_tls_verify": true,
			},
			map[string]any{"name": "component", "url": "https://component.example.com"},
		},
	}

	got := mergeRepositories(atmosConfig, section)
	require.Len(t, got, 3)
	assert.Equal(t, "global", got[0].Name)
	assert.Equal(t, repositorySourceGlobal, got[0].Source)
	assert.Equal(t, "shared", got[1].Name)
	assert.Equal(t, "https://new.example.com", got[1].URL)
	assert.Equal(t, repositorySourceComponent, got[1].Source)
	assert.Equal(t, "user", got[1].Username)
	assert.Equal(t, "pass", got[1].Password)
	assert.True(t, got[1].PassCredentialsAll)
	assert.Equal(t, "cert.pem", got[1].CertFile)
	assert.Equal(t, "key.pem", got[1].KeyFile)
	assert.Equal(t, "ca.pem", got[1].CAFile)
	assert.True(t, got[1].InsecureSkipTLSVerify)
	assert.Equal(t, "component", got[2].Name)
}

func TestSetupHelmRepositoriesWritesConfigAndDownloadsIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/index.yaml", r.URL.Path)
		_, _ = w.Write([]byte(`apiVersion: v1
entries:
  nginx:
    - apiVersion: v2
      name: nginx
      version: 1.0.0
generated: "2026-06-30T00:00:00Z"
`))
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	repoFile := filepath.Join(dir, "repositories.yaml")
	repoCache := filepath.Join(dir, "repository")
	t.Setenv("HELM_REPOSITORY_CONFIG", repoFile)
	t.Setenv("HELM_REPOSITORY_CACHE", repoCache)

	err := setupHelmRepositories([]chartRepository{
		{Name: "example", URL: server.URL},
	})
	require.NoError(t, err)

	loaded, err := repo.LoadFile(repoFile)
	require.NoError(t, err)
	entry := loaded.Get("example")
	require.NotNil(t, entry)
	assert.Equal(t, server.URL, entry.URL)
	assert.FileExists(t, filepath.Join(repoCache, "example-index.yaml"))

	err = setupHelmRepositories([]chartRepository{
		{Name: "example", URL: server.URL, Username: "next"},
	})
	require.NoError(t, err)
	loaded, err = repo.LoadFile(repoFile)
	require.NoError(t, err)
	entry = loaded.Get("example")
	require.NotNil(t, entry)
	assert.Equal(t, "next", entry.Username)
}

func TestSetupHelmRepositories_LockUnavailable(t *testing.T) {
	// Root ignores the 0o000 permission bits below, so the lock would actually be
	// acquired and setupHelmRepositories would proceed to a real request against
	// the "https://example.com" URL used below; Windows uses a best-effort no-op
	// FileLock (see pkg/cache/filelock_windows.go) that never returns ErrCacheLocked.
	// Both are covered by the shared skip helper.
	skipIfCannotDenyDirWrite(t)

	dir := t.TempDir()
	repoFile := filepath.Join(dir, "repositories.yaml")
	repoCache := filepath.Join(dir, "repository")
	t.Setenv("HELM_REPOSITORY_CONFIG", repoFile)
	t.Setenv("HELM_REPOSITORY_CACHE", repoCache)

	// setupHelmRepositories always creates repoFile's parent directory first, so a
	// missing-parent-directory trick can't make *opening* the lock file fail. Instead,
	// pre-create the lock file with no permission bits: opening it for the flock
	// handshake then fails immediately with "permission denied" instead of retrying
	// until repositoryLockTimeout elapses.
	require.NoError(t, os.MkdirAll(filepath.Dir(repoFile), os.ModePerm))
	lockPath := repositoryLockPath(repoFile)
	require.NoError(t, os.WriteFile(lockPath, nil, 0o000))
	t.Cleanup(func() { _ = os.Chmod(lockPath, 0o600) })

	err := setupHelmRepositories([]chartRepository{
		{Name: "example", URL: "https://example.com"},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errHelmRepositoryLockTimeout))
}

func TestRepositoryEntryRejectsSlashName(t *testing.T) {
	_, err := repositoryEntry(&chartRepository{Name: "bad/name", URL: "https://example.com"})
	require.Error(t, err)
}

func TestLoadRepositoryFileCreatesEmptyWhenMissing(t *testing.T) {
	file, err := loadRepositoryFile(filepath.Join(t.TempDir(), "missing.yaml"))
	require.NoError(t, err)
	require.NotNil(t, file)
	assert.Empty(t, file.Repositories)
}

func TestRepositoryLockPath(t *testing.T) {
	assert.Equal(t, "repositories.lock", repositoryLockPath("repositories.yaml"))
	assert.Equal(t, "repositories.lock", repositoryLockPath("repositories"))
}

func TestLoadRepositoryFileRejectsInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repositories.yaml")
	require.NoError(t, os.WriteFile(path, []byte("repositories: ["), 0o600))
	_, err := loadRepositoryFile(path)
	require.Error(t, err)
}

// TestSetupHelmRepositories_NoRepositoriesReturnsNil verifies the early-return
// guard: with no declarative repositories, setupHelmRepositories must do
// nothing (no settings resolution, no directory creation).
func TestSetupHelmRepositories_NoRepositoriesReturnsNil(t *testing.T) {
	err := setupHelmRepositories(nil)
	assert.NoError(t, err)

	err = setupHelmRepositories([]chartRepository{})
	assert.NoError(t, err)
}

// TestSetupHelmRepositories_EmptyRepoFileConfigReturnsNil verifies that an empty
// RepositoryConfig short-circuits before any filesystem access, per the
// "if repoFile == \"\" { return nil }" guard. Because newSettings now isolates the
// repository config to an atmos-managed path (issue #1), the empty config is produced by
// stubbing newSettings rather than by an empty HELM_REPOSITORY_CONFIG env var.
func TestSetupHelmRepositories_EmptyRepoFileConfigReturnsNil(t *testing.T) {
	orig := newSettings
	t.Cleanup(func() { newSettings = orig })
	newSettings = func() *cli.EnvSettings {
		s := cli.New()
		s.RepositoryConfig = ""
		return s
	}

	err := setupHelmRepositories([]chartRepository{{Name: "example", URL: "https://example.com"}})
	assert.NoError(t, err)
}

// TestSetupHelmRepositories_MkdirAllFailure verifies that a MkdirAll failure
// (the repository file's parent directory already exists as a regular file)
// is surfaced instead of silently ignored.
func TestSetupHelmRepositories_MkdirAllFailure(t *testing.T) {
	dir := t.TempDir()
	// "config-parent" will be used as the parent directory of the repo file,
	// but it's created as a plain file, so MkdirAll must fail with ENOTDIR.
	blockingFile := filepath.Join(dir, "config-parent")
	require.NoError(t, os.WriteFile(blockingFile, []byte("not a directory"), 0o600))
	repoFile := filepath.Join(blockingFile, "repositories.yaml")
	t.Setenv("HELM_REPOSITORY_CONFIG", repoFile)

	err := setupHelmRepositories([]chartRepository{{Name: "example", URL: "https://example.com"}})
	require.Error(t, err)
}

// TestUpdateRepositories_LoadRepositoryFileFailure verifies that
// updateRepositories propagates a loadRepositoryFile failure (corrupted
// existing YAML) instead of overwriting it. The updateRepositories helper
// runs inside setupHelmRepositories' lock closure, so it's exercised
// directly here rather than through the lock.
func TestUpdateRepositories_LoadRepositoryFileFailure(t *testing.T) {
	repoFile := filepath.Join(t.TempDir(), "repositories.yaml")
	require.NoError(t, os.WriteFile(repoFile, []byte("repositories: ["), 0o600))

	settings := &cli.EnvSettings{}
	err := updateRepositories(settings, repoFile, []chartRepository{{Name: "example", URL: "https://example.com"}})
	require.Error(t, err)
}

// TestUpdateRepositories_InvalidRepositoryName verifies that
// updateRepositories propagates the repositoryEntry validation error for a
// name containing '/'.
func TestUpdateRepositories_InvalidRepositoryName(t *testing.T) {
	repoFile := filepath.Join(t.TempDir(), "repositories.yaml")

	settings := &cli.EnvSettings{}
	err := updateRepositories(settings, repoFile, []chartRepository{{Name: "bad/name", URL: "https://example.com"}})
	require.Error(t, err)
	assert.ErrorIs(t, err, errInvalidHelmRepositoryName)
}

// TestUpdateRepositories_UnsupportedSchemeFailure verifies that
// updateRepositories surfaces repo.NewChartRepository's error when the
// repository URL uses a scheme with no registered getter (e.g. "ftp").
func TestUpdateRepositories_UnsupportedSchemeFailure(t *testing.T) {
	repoFile := filepath.Join(t.TempDir(), "repositories.yaml")

	settings := cli.New()
	err := updateRepositories(settings, repoFile, []chartRepository{{Name: "example", URL: "ftp://example.com/charts"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "protocol handler")
}

// TestUpdateRepositories_DownloadIndexFailure verifies that an unreachable
// repository URL is surfaced with the "not a valid chart repository or
// cannot be reached" wrapper rather than a bare transport error.
func TestUpdateRepositories_DownloadIndexFailure(t *testing.T) {
	repoFile := filepath.Join(t.TempDir(), "repositories.yaml")

	// Connect to a port nothing is listening on so the request fails fast
	// with "connection refused" instead of waiting out a DNS/route timeout.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	unreachableURL := "http://" + listener.Addr().String()
	require.NoError(t, listener.Close()) // Close immediately: nothing listens on this port now.

	settings := cli.New()
	err = updateRepositories(settings, repoFile, []chartRepository{{Name: "example", URL: unreachableURL}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid chart repository or cannot be reached")
}

// TestUpdateRepositories_WriteFileFailure verifies that a WriteFile failure
// (the repo file's directory made read-only) is surfaced rather than
// silently discarded after a successful download.
func TestUpdateRepositories_WriteFileFailure(t *testing.T) {
	skipIfCannotDenyDirWrite(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`apiVersion: v1
entries: {}
generated: "2026-06-30T00:00:00Z"
`))
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	repoFile := filepath.Join(dir, "repositories.yaml")
	require.NoError(t, os.Chmod(dir, 0o500)) // Read-only: blocks creating the new repositories.yaml.
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	settings := cli.New()
	settings.RepositoryCache = t.TempDir()
	err := updateRepositories(settings, repoFile, []chartRepository{{Name: "example", URL: server.URL}})
	require.Error(t, err)
}

// TestSetupHelmRepositories_PropagatesNonLockError verifies that once the
// lock is successfully acquired, a non-ErrCacheLocked failure from
// updateRepositories (an invalid repository name) is returned as-is rather
// than being mistaken for a lock-timeout error.
func TestSetupHelmRepositories_PropagatesNonLockError(t *testing.T) {
	dir := t.TempDir()
	repoFile := filepath.Join(dir, "repositories.yaml")
	t.Setenv("HELM_REPOSITORY_CONFIG", repoFile)
	t.Setenv("HELM_REPOSITORY_CACHE", filepath.Join(dir, "repository"))

	err := setupHelmRepositories([]chartRepository{{Name: "bad/name", URL: "https://example.com"}})
	require.Error(t, err)
	assert.ErrorIs(t, err, errInvalidHelmRepositoryName)
	assert.False(t, errors.Is(err, errHelmRepositoryLockTimeout), "a validation error must not be mistaken for a lock timeout")
}

// TestLoadRepositoryFileReadFailure verifies the non-ENOENT os.ReadFile
// failure branch (the path is a directory, not a missing file).
func TestLoadRepositoryFileReadFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repositories.yaml")
	require.NoError(t, os.MkdirAll(path, 0o700)) // Directory in place of the file.

	_, err := loadRepositoryFile(path)
	require.Error(t, err)
}

// TestLoadRepositoryFileDefaultsNilRepositories verifies that a valid YAML
// file which simply omits the "repositories" key ends up with an empty,
// non-nil slice rather than nil (callers call .Update/.Has on it directly).
func TestLoadRepositoryFileDefaultsNilRepositories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repositories.yaml")
	require.NoError(t, os.WriteFile(path, []byte("apiVersion: v1\n"), 0o600))

	file, err := loadRepositoryFile(path)
	require.NoError(t, err)
	require.NotNil(t, file)
	assert.NotNil(t, file.Repositories, "Repositories must be defaulted to an empty slice, not left nil")
	assert.Empty(t, file.Repositories)
}
