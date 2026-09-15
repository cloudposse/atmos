package plugin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

var pinnedDiff = Spec{Name: "diff", URL: diffRepository, Version: "v3.9.4"}

func TestInstallRetry(t *testing.T) {
	for _, tc := range []struct {
		name         string
		failures     int
		stderr       string
		wantAttempts int
		wantError    bool
	}{
		{"recovery", 2, "curl: (22) The requested URL returned error: 500", 3, false},
		{"exhaustion", 3, "HTTP 503 Service Unavailable", 3, true},
		{"permanent", 3, "HTTP 404 Not Found", 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "plugins")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			previous := filepath.Join(dir, "keep")
			require.NoError(t, os.WriteFile(previous, []byte("previous plugin"), 0o644))
			attempts := 0
			var stages []string
			runner := &fakeRunner{listOutput: "NAME VERSION\ndiff 3.8.0\n"}
			runner.hook = func(_ context.Context, args []string, stage string) (bool, string, string, error) {
				if args[0] != "plugin" || args[1] != "install" {
					return false, "", "", nil
				}
				attempts++
				entries, err := os.ReadDir(stage)
				require.NoError(t, err)
				require.Empty(t, entries, "every attempt must start clean")
				for _, old := range stages {
					require.NoDirExists(t, old)
				}
				stages = append(stages, stage)
				if attempts <= tc.failures {
					require.NoError(t, os.WriteFile(filepath.Join(stage, "partial"), []byte("debris"), 0o644))
					return true, "", tc.stderr, errors.New("exit status 22")
				}
				return false, "", "", nil
			}
			_, err := newTestInstaller(runner, dir).EnsurePlugins(context.Background(), []Spec{pinnedDiff})
			if tc.wantError {
				require.ErrorIs(t, err, errUtils.ErrHelmPluginInstall)
				require.Empty(t, runner.uninstallCalls(), "failed replacement must keep existing plugin")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantAttempts, attempts)
			require.FileExists(t, previous)
			for _, stage := range stages {
				require.NoDirExists(t, stage)
			}
		})
	}
}

func TestInstallRetryDefaults(t *testing.T) {
	config := defaultInstallRetryConfig()
	assert.Equal(t, 3, *config.MaxAttempts)
	assert.Equal(t, 15*time.Second, *config.InitialDelay)
	assert.Equal(t, 30*time.Second, *config.MaxDelay)
	assert.Equal(t, schema.BackoffExponential, config.BackoffStrategy)
}

func TestInstallCancellation(t *testing.T) {
	t.Run("before attempt", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		runner := &fakeRunner{}
		err := newTestInstaller(runner, t.TempDir()).install(ctx, pinnedDiff, "")
		require.ErrorIs(t, err, context.Canceled)
		require.Empty(t, runner.calls)
	})
	t.Run("during backoff", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		runner := &fakeRunner{hook: func(_ context.Context, args []string, _ string) (bool, string, string, error) {
			if args[1] != "install" {
				return false, "", "", nil
			}
			cancel()
			return true, "", "HTTP 500", errors.New("exit status 22")
		}}
		inst := newTestInstaller(runner, t.TempDir())
		inst.retryConfig = defaultInstallRetryConfig()
		require.ErrorIs(t, inst.install(ctx, pinnedDiff, ""), context.Canceled)
		require.Len(t, runner.installCalls(), 1)
	})
	t.Run("before publish", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		runner := &fakeRunner{hook: func(_ context.Context, args []string, _ string) (bool, string, string, error) {
			if args[0] == "diff" {
				cancel()
				return true, "3.9.4", "", nil
			}
			return false, "", "", nil
		}}
		dir := t.TempDir()
		require.ErrorIs(t, newTestInstaller(runner, dir).install(ctx, pinnedDiff, "diff"), context.Canceled)
		require.Empty(t, runner.uninstallCalls())
		require.NoDirExists(t, filepath.Join(dir, "diff"))
	})
}

func TestCachedDiffBinaryIsVerifiedAndRepaired(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		err          error
	}{
		{"wrong version", "3.15.13", nil},
		{"missing executable", "", os.ErrNotExist},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			runner := &fakeRunner{listOutput: "NAME VERSION\ndiff 3.9.4\n"}
			runner.hook = func(_ context.Context, args []string, plugins string) (bool, string, string, error) {
				if args[0] == "diff" && plugins == dir {
					return true, tc.output, "missing executable", tc.err
				}
				return false, "", "", nil
			}
			_, err := newTestInstaller(runner, dir).EnsurePlugins(context.Background(), []Spec{pinnedDiff})
			require.NoError(t, err)
			require.Len(t, runner.installCalls(), 1)
		})
	}
}

func TestInvalidInstallIsNotPublished(t *testing.T) {
	for _, tc := range []struct {
		name, metadata, binary string
		verifyErr              error
	}{
		{name: "missing metadata"},
		{name: "invalid yaml", metadata: "name: ["},
		{name: "missing name", metadata: "version: 3.9.4"},
		{name: "wrong metadata version", metadata: "name: diff\nversion: 3.15.13"},
		{name: "wrong binary version", metadata: "name: diff\nversion: 3.9.4", binary: "3.15.13"},
		{name: "broken executable", metadata: "name: diff\nversion: 3.9.4", verifyErr: os.ErrPermission},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			runner := &fakeRunner{hook: func(_ context.Context, args []string, stage string) (bool, string, string, error) {
				if args[0] == "diff" {
					return true, tc.binary, "cannot execute", tc.verifyErr
				}
				if args[1] != "install" {
					return false, "", "", nil
				}
				if tc.metadata != "" {
					require.NoError(t, os.MkdirAll(filepath.Join(stage, "diff"), 0o755))
					require.NoError(t, os.WriteFile(filepath.Join(stage, "diff", "plugin.yaml"), []byte(tc.metadata), 0o644))
				}
				return true, "", "", nil
			}}
			_, err := newTestInstaller(runner, dir).EnsurePlugins(context.Background(), []Spec{pinnedDiff})
			require.ErrorIs(t, err, errUtils.ErrHelmPluginInstall)
			require.Len(t, runner.installCalls(), 1)
			require.NoDirExists(t, filepath.Join(dir, "diff"))
		})
	}
}

func TestInstallFilesystemAndHelmErrors(t *testing.T) {
	t.Run("staging parent missing", func(t *testing.T) {
		inst := newTestInstaller(&fakeRunner{}, filepath.Join(t.TempDir(), "missing", "plugins"))
		require.ErrorContains(t, inst.install(context.Background(), pinnedDiff, ""), "create staging directory")
	})
	t.Run("publish collision", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "diff"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "diff", "keep"), nil, 0o644))
		inst := newTestInstaller(&fakeRunner{}, dir)
		require.ErrorContains(t, inst.install(context.Background(), pinnedDiff, ""), "publish installed plugin")
	})
	for _, operation := range []string{"list", "uninstall"} {
		t.Run(operation, func(t *testing.T) {
			runner := &fakeRunner{listOutput: "NAME VERSION\ndiff 3.8.0\n"}
			runner.hook = func(_ context.Context, args []string, _ string) (bool, string, string, error) {
				if args[0] == "plugin" && args[1] == operation {
					return true, "", "permission denied", os.ErrPermission
				}
				return false, "", "", nil
			}
			_, err := newTestInstaller(runner, t.TempDir()).EnsurePlugins(context.Background(), []Spec{pinnedDiff})
			require.ErrorIs(t, err, errUtils.ErrHelmPluginInstall)
		})
	}
	t.Run("managed path is a file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(path, nil, 0o644))
		_, err := newTestInstaller(&fakeRunner{}, path).EnsurePlugins(context.Background(), []Spec{pinnedDiff})
		require.ErrorIs(t, err, errUtils.ErrHelmPluginInstall)
	})
}

func TestInstallURL(t *testing.T) {
	for _, goos := range []string{"linux", "windows", "darwin", "freebsd"} {
		for _, arch := range []string{"amd64", "arm64"} {
			osName := goos
			if goos == "darwin" {
				osName = "macos"
			}
			want := fmt.Sprintf("%s/releases/download/v3.9.4/helm-diff-%s-%s.tgz", diffRepository, osName, arch)
			assert.Equal(t, want, installURL(pinnedDiff, goos, arch))
		}
	}
	for _, spec := range []Spec{
		{URL: diffRepository},
		{URL: diffRepository, Version: "main"},
		{URL: "https://example.com/helm-diff", Version: "v3.9.4"},
	} {
		assert.Equal(t, spec.URL, installURL(spec, "windows", "amd64"))
	}
	assert.Equal(t, pinnedDiff.URL, installURL(pinnedDiff, "plan9", "amd64"))
	assert.Equal(t, pinnedDiff.URL, installURL(pinnedDiff, "linux", "386"))
	assert.Equal(t, installURL(pinnedDiff, "linux", "amd64"), installURL(Spec{URL: diffRepository + ".git/", Version: "3.9.4"}, "linux", "amd64"))
}

func TestTransientInstallErrors(t *testing.T) {
	for _, message := range []string{"HTTP 429", "HTTP/2 502", "status code: 504", "error: 500", "could not resolve host", "no such host", "temporary failure in name resolution", "connection reset", "connection refused", "connection timed out", "i/o timeout", "tls handshake timeout", "unexpected EOF"} {
		assert.True(t, isTransientInstallError(errors.New(message)), message)
	}
	for _, err := range []error{nil, context.Canceled, context.DeadlineExceeded, errors.New("HTTP 404"), errors.New("version mismatch"), errors.New("permission denied")} {
		assert.False(t, isTransientInstallError(err))
	}
}

func TestConcurrentEnsureInstallsOnce(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeRunner{}
	runner.hook = func(_ context.Context, args []string, plugins string) (bool, string, string, error) {
		if args[0] == "plugin" && args[1] == "list" {
			if _, err := os.Stat(filepath.Join(plugins, "diff", "plugin.yaml")); err == nil {
				return true, "NAME VERSION\ndiff 3.9.4\n", "", nil
			}
		}
		return false, "", "", nil
	}
	var wg sync.WaitGroup
	results := make(chan error, 4)
	for range 4 {
		wg.Go(func() {
			_, err := newTestInstaller(runner, dir).EnsurePlugins(context.Background(), []Spec{pinnedDiff, pinnedDiff})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	require.Len(t, runner.installCalls(), 1)
}

func TestValidateInstallReadFailure(t *testing.T) {
	stage := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(stage, "diff", "plugin.yaml"), 0o755))
	_, err := validateInstall(stage, pinnedDiff)
	require.ErrorContains(t, err, "read plugin metadata")
}

func TestVerifyDiffWindowsOutput(t *testing.T) {
	runner := &fakeRunner{hook: func(_ context.Context, _ []string, _ string) (bool, string, string, error) {
		return true, "v3.9.4\r\n", "", nil
	}}
	require.NoError(t, newTestInstaller(runner, t.TempDir()).verifyDiff(context.Background(), pinnedDiff, "plugins"))
}

func TestInstallNoStagingLeak(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "plugins")
	_, err := newTestInstaller(&fakeRunner{}, dir).EnsurePlugins(context.Background(), []Spec{pinnedDiff})
	require.NoError(t, err)
	entries, err := os.ReadDir(parent)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), ".helm-plugin-install-"))
	}
}

func TestDownloadDiffRelease(t *testing.T) {
	var archive bytes.Buffer
	compressed := gzip.NewWriter(&archive)
	writer := tar.NewWriter(compressed)
	for name, data := range map[string]string{"diff/plugin.yaml": "name: diff\nversion: 3.9.4", "diff/bin/diff": "binary contents"} {
		require.NoError(t, writer.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data))}))
		_, err := writer.Write([]byte(data))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	require.NoError(t, compressed.Close())
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if strings.HasPrefix(r.URL.Path, "/failure") {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/corrupt") {
			_, _ = w.Write([]byte("not gzip"))
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(archive.Bytes())
	}))
	defer server.Close()
	stage := t.TempDir()
	require.NoError(t, downloadDiffRelease(context.Background(), server.URL+"/plugin.tgz", stage))
	pluginDir, err := validateInstall(stage, pinnedDiff)
	require.NoError(t, err)
	binary := filepath.Join(pluginDir, "bin", "diff")
	data, err := os.ReadFile(binary)
	require.NoError(t, err)
	require.Equal(t, "binary contents", string(data))
	if runtime.GOOS != "windows" {
		info, err := os.Stat(binary)
		require.NoError(t, err)
		require.NotZero(t, info.Mode()&0o111)
	}
	err = downloadDiffRelease(context.Background(), server.URL+"/failure.tgz", t.TempDir())
	require.Error(t, err)
	require.True(t, isTransientInstallError(err), err.Error())
	require.Error(t, downloadDiffRelease(context.Background(), server.URL+"/corrupt.tgz", t.TempDir()))
	before := requests.Load()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, downloadDiffRelease(ctx, server.URL+"/plugin.tgz", t.TempDir()))
	require.Equal(t, before, requests.Load())
}
