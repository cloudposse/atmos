package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	atmosversion "github.com/cloudposse/atmos/pkg/version"
	"github.com/cloudposse/atmos/tests/testhelpers/httpmock"
)

func TestCIVersionNoticeCommandLifecycle(t *testing.T) {
	for _, disabled := range []string{"", "ATMOS_VERSION_CHECK_ENABLED", "ATMOS_VERSION_CHECK_CI_ENABLED"} {
		t.Run("disabled="+disabled, func(t *testing.T) {
			_ = NewTestKit(t)
			directory := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(directory, "atmos.yaml"), []byte("version:\n  check:\n    enabled: true\n    timeout: 2000\n    ci:\n      enabled: true\n"), 0o600))
			t.Setenv("ATMOS_CLI_CONFIG_PATH", directory)
			t.Setenv("ATMOS_BASE_PATH", directory)
			t.Setenv("ATMOS_VERSION_CHECK_ENABLED", "true")
			t.Setenv("ATMOS_VERSION_CHECK_CI_ENABLED", "true")
			if disabled != "" {
				t.Setenv(disabled, "false")
			}
			t.Setenv("ATMOS_CI_VERSION_NOTICE_SHOWN", "")
			t.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
			t.Setenv("ATMOS_PRO_ENABLED", "false")
			t.Setenv("GITHUB_ACTIONS", "true")
			t.Setenv("RUNNER_TEMP", t.TempDir())
			t.Setenv("GITHUB_ENV", filepath.Join(t.TempDir(), "env"))
			t.Setenv("GITHUB_RUN_ID", "123")
			t.Setenv("GITHUB_RUN_ATTEMPT", "1")
			t.Setenv("GITHUB_JOB", "notice-test")
			registerFreshGHAProvider(t)
			mock := httpmock.NewGitHubMockServer(t)
			handler := mock.Server.Config.Handler
			mock.Server.Close()
			mock.Server = httptest.NewTLSServer(handler)
			t.Cleanup(mock.Server.Close)
			transport := http.DefaultTransport
			http.DefaultTransport = mock.Server.Client().Transport
			t.Cleanup(func() { http.DefaultTransport = transport })
			mock.Setenv(t)
			mock.RegisterRelease("cloudposse", "atmos", httpmock.ReleaseSpec{TagName: "v1.110.0"})
			t.Setenv("ATMOS_PRO_GITHUB_TOKEN", "")
			t.Setenv("ATMOS_GITHUB_TOKEN", "test-token")
			t.Setenv("GITHUB_TOKEN", "")
			t.Setenv("ATMOS_GITHUB_CLI", "")
			previousVersion := atmosversion.Version
			atmosversion.Version = "1.100.0"
			t.Cleanup(func() { atmosversion.Version = previousVersion })
			previousArgs := os.Args
			os.Args = []string{"atmos", "ci-notice-fixture"}
			t.Cleanup(func() { os.Args = previousArgs })
			RootCmd.SetArgs(os.Args[1:])
			RootCmd.AddCommand(&cobra.Command{
				Use: "ci-notice-fixture",
				// Keep the fixture focused on Execute's notice lifecycle rather than provider startup banners.
				PersistentPreRun: func(*cobra.Command, []string) {},
				RunE: func(*cobra.Command, []string) error {
					if disabled == "" {
						require.Eventually(t, func() bool {
							cache, err := cfg.LoadCache()
							return err == nil && cache.LastChecked > 0
						}, 3*time.Second, 10*time.Millisecond)
						// Windows cache writes defer a 50 ms handle-release delay after
						// the timestamp becomes visible. Keep this fixture command alive
						// beyond that delay so the worker can publish its result.
						time.Sleep(100 * time.Millisecond)
					}
					return data.Writeln(`{"ok":true}`)
				},
			})
			stdout, stderr := captureStdoutStderr(t, func() {
				ensureIOInitialized(t)
				require.NoError(t, Execute())
			})
			assert.JSONEq(t, `{"ok":true}`, stdout)
			if disabled == "" {
				assert.Contains(t, stderr, "::warning title=Atmos 1.100.0 is 10 releases behind::Latest is 1.110.0.")
				assert.Len(t, mock.Requests(), 2)
			} else {
				assert.NotContains(t, stderr, "::warning")
				assert.Empty(t, mock.Requests(), "disabled checks must not contact GitHub")
			}
		})
	}
}
