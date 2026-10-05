package plugin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

var testPlugin = Spec{Name: "sample", URL: "https://example.com/helm-sample", Version: "v1.2.3"}

func seedPlugin(t *testing.T, dir, name, version string, receipt *Spec) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(path, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(path, "plugin.yaml"), []byte("name: "+name+"\nversion: "+version+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(path, "binary"), []byte("previous binary"), 0o755))
	if receipt != nil {
		require.NoError(t, writeInstallReceipt(path, *receipt))
	}
	return path
}

func TestInstallRetriesCleanOnlyNewEntries(t *testing.T) {
	for _, tc := range []struct {
		name, message      string
		failures, attempts int
		wantError          bool
	}{
		{"recovery", "HTTP 500", 2, 3, false},
		{"exhaustion", "HTTP 503", 3, 3, true},
		{"permanent", "HTTP 404", 3, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			keep := seedPlugin(t, dir, "unrelated", "1.0.0", nil)
			attempts := 0
			runner := &fakeRunner{hook: func(_ context.Context, args []string, plugins string) (bool, string, string, error) {
				if args[0] != "plugin" || args[1] != "install" {
					return false, "", "", nil
				}
				attempts++
				require.Equal(t, dir, plugins, "hooks must use their final installation directory")
				require.NoDirExists(t, filepath.Join(plugins, "sample"))
				require.NoFileExists(t, filepath.Join(plugins, "partial"))
				require.FileExists(t, filepath.Join(keep, "binary"))
				if attempts <= tc.failures {
					seedPlugin(t, plugins, "sample", "1.2.3", nil)
					require.NoError(t, os.WriteFile(filepath.Join(plugins, "partial"), nil, 0o644))
					return true, "", tc.message, errors.New("exit status 22")
				}
				return false, "", "", nil
			}}
			inst := newTestInstaller(runner, dir)
			_, err := inst.EnsurePlugins(context.Background(), []Spec{testPlugin})
			if tc.wantError {
				require.ErrorIs(t, err, errUtils.ErrHelmPluginInstall)
				require.NoDirExists(t, filepath.Join(dir, "sample"))
			} else {
				require.NoError(t, err)
				assert.True(t, inst.hasInstallReceipt("sample", testPlugin))
			}
			assert.Equal(t, tc.attempts, attempts)
			require.FileExists(t, filepath.Join(keep, "binary"))
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
		require.ErrorIs(t, newTestInstaller(runner, t.TempDir()).install(ctx, testPlugin, ""), context.Canceled)
		assert.Empty(t, runner.calls)
	})
	t.Run("during backoff", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		runner := &fakeRunner{hook: func(_ context.Context, _ []string, _ string) (bool, string, string, error) {
			time.AfterFunc(20*time.Millisecond, cancel)
			return true, "", "HTTP 503", os.ErrInvalid
		}}
		inst := newTestInstaller(runner, t.TempDir())
		inst.retryConfig = defaultInstallRetryConfig()
		require.ErrorIs(t, inst.install(ctx, testPlugin, ""), context.Canceled)
		require.Len(t, runner.installCalls(), 1)
	})
	t.Run("during hook", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dir := t.TempDir()
		runner := &fakeRunner{hook: func(_ context.Context, args []string, plugins string) (bool, string, string, error) {
			if args[1] != "install" {
				return false, "", "", nil
			}
			seedPlugin(t, plugins, "sample", "1.2.3", nil)
			cancel()
			return true, "", "", nil
		}}
		require.ErrorIs(t, newTestInstaller(runner, dir).install(ctx, testPlugin, ""), context.Canceled)
		require.NoDirExists(t, filepath.Join(dir, "sample"))
	})
}

func TestIncompleteInstallationIsRepaired(t *testing.T) {
	for _, receipt := range []string{"", "invalid: [", "source: wrong\nversion: v1.2.3"} {
		dir := t.TempDir()
		existing := seedPlugin(t, dir, "sample", "1.2.3", nil)
		if receipt != "" {
			require.NoError(t, os.WriteFile(filepath.Join(existing, installReceiptName), []byte(receipt), 0o644))
		}
		runner := &fakeRunner{}
		inst := newTestInstaller(runner, dir)
		_, err := inst.EnsurePlugins(context.Background(), []Spec{testPlugin, testPlugin})
		require.NoError(t, err)
		require.Len(t, runner.installCalls(), 1)
		require.Len(t, runner.uninstallCalls(), 1)
		assert.True(t, inst.hasInstallReceipt("sample", testPlugin))
	}
}

func TestInvalidInstallIsCleaned(t *testing.T) {
	for _, metadata := range []string{"", "name: [", "version: 1.2.3", "name: sample\nversion: 9.0.0"} {
		dir := t.TempDir()
		runner := &fakeRunner{hook: func(_ context.Context, args []string, plugins string) (bool, string, string, error) {
			if args[1] != "install" {
				return false, "", "", nil
			}
			if metadata != "" {
				path := seedPlugin(t, plugins, "sample", "1.2.3", nil)
				require.NoError(t, os.WriteFile(filepath.Join(path, "plugin.yaml"), []byte(metadata), 0o644))
			}
			return true, "", "", nil
		}}
		_, err := newTestInstaller(runner, dir).EnsurePlugins(context.Background(), []Spec{testPlugin})
		require.ErrorIs(t, err, errUtils.ErrHelmPluginInstall)
		require.Len(t, runner.installCalls(), 1)
		require.NoDirExists(t, filepath.Join(dir, "sample"))
	}
}

func TestPluginInstallationErrors(t *testing.T) {
	t.Run("cannot create directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(path, nil, 0o644))
		_, err := newTestInstaller(&fakeRunner{}, path).EnsurePlugins(context.Background(), []Spec{testPlugin})
		require.ErrorIs(t, err, errUtils.ErrHelmPluginInstall)
	})
	t.Run("list failure", func(t *testing.T) {
		runner := &fakeRunner{hook: func(_ context.Context, _ []string, _ string) (bool, string, string, error) {
			return true, "", "denied", os.ErrPermission
		}}
		_, err := newTestInstaller(runner, t.TempDir()).EnsurePlugins(context.Background(), []Spec{testPlugin})
		require.ErrorIs(t, err, errUtils.ErrHelmPluginInstall)
	})
	t.Run("missing directory", func(t *testing.T) {
		inst := newTestInstaller(&fakeRunner{}, filepath.Join(t.TempDir(), "missing"))
		require.ErrorIs(t, inst.installAttempt(context.Background(), testPlugin, ""), errUtils.ErrHelmPluginInstall)
	})
	t.Run("receipt failure", func(t *testing.T) {
		dir := t.TempDir()
		runner := &fakeRunner{hook: func(_ context.Context, args []string, plugins string) (bool, string, string, error) {
			if args[1] != "install" {
				return false, "", "", nil
			}
			path := seedPlugin(t, plugins, "sample", "1.2.3", nil)
			require.NoError(t, os.Mkdir(filepath.Join(path, installReceiptName), 0o755))
			return true, "", "", nil
		}}
		_, err := newTestInstaller(runner, dir).EnsurePlugins(context.Background(), []Spec{testPlugin})
		require.ErrorContains(t, err, "record successful installation")
		require.NoDirExists(t, filepath.Join(dir, "sample"))
	})
}

func TestConcurrentEnsureInstallsOnce(t *testing.T) {
	dir := t.TempDir()
	runner := &fakeRunner{}
	var wg sync.WaitGroup
	results := make(chan error, 4)
	for range 4 {
		wg.Go(func() {
			_, err := newTestInstaller(runner, dir).EnsurePlugins(context.Background(), []Spec{testPlugin})
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

func TestTransientInstallErrors(t *testing.T) {
	for _, message := range []string{"HTTP 429", "HTTP/2 502", "response code: 503", "status code: 504", "error: 500", "could not resolve host", "no such host", "temporary failure in name resolution", "connection reset", "connection refused", "connection timed out", "i/o timeout", "tls handshake timeout", "unexpected EOF"} {
		assert.True(t, isTransientInstallError(errors.New(message)), message)
	}
	for _, err := range []error{nil, context.Canceled, context.DeadlineExceeded, errors.Join(errors.New("HTTP 503"), errInstallCleanup), errors.New("HTTP 404"), errors.New("version mismatch")} {
		assert.False(t, isTransientInstallError(err))
	}
}

func TestCustomPluginNameUsesReceiptIdentity(t *testing.T) {
	runner := &fakeRunner{pluginName: "different-name"}
	inst := newTestInstaller(runner, t.TempDir())
	_, err := inst.EnsurePlugins(context.Background(), []Spec{testPlugin, testPlugin})
	require.NoError(t, err)
	require.Len(t, runner.installCalls(), 1)
	updated := testPlugin
	updated.Version = "v2.0.0"
	_, err = inst.EnsurePlugins(context.Background(), []Spec{updated})
	require.NoError(t, err)
	require.Len(t, runner.installCalls(), 2)
	require.Len(t, runner.uninstallCalls(), 1)
	assert.True(t, inst.hasInstallReceipt("different-name", updated))
}

func TestPluginIdentityValidation(t *testing.T) {
	for _, tc := range []struct {
		spec     Spec
		expected string
		metadata pluginMetadata
	}{
		{Spec{Name: "diff", URL: "https://github.com/databus23/helm-diff"}, "", pluginMetadata{Name: "wrong"}},
		{testPlugin, "previous-name", pluginMetadata{Name: "different-name"}},
	} {
		require.ErrorIs(t, validatePluginMetadata(tc.metadata, tc.spec, tc.expected), errUtils.ErrHelmPluginInstall)
	}
}
