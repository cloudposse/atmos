package emulator

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
)

// fakeGiteaExecer is a giteaExecer stub: it records the last command/options and
// returns a configured error, optionally writing canned text to the exec streams.
type fakeGiteaExecer struct {
	err     error
	stdout  string
	stderr  string
	lastCmd []string
	lastOpt *container.ExecOptions
}

func (f *fakeGiteaExecer) Exec(_ context.Context, _ string, cmd []string, opts *container.ExecOptions) error {
	f.lastCmd = cmd
	f.lastOpt = opts
	if opts != nil {
		if opts.Stdout != nil && f.stdout != "" {
			_, _ = io.WriteString(opts.Stdout, f.stdout)
		}
		if opts.Stderr != nil && f.stderr != "" {
			_, _ = io.WriteString(opts.Stderr, f.stderr)
		}
	}
	return f.err
}

func TestEnsureGiteaAdmin(t *testing.T) {
	t.Run("creates admin as git user against the installed config", func(t *testing.T) {
		exec := &fakeGiteaExecer{}
		require.NoError(t, ensureGiteaAdmin(context.Background(), exec, "cid"))

		// Command must target the admin-create CLI with the throwaway creds + config.
		joined := strings.Join(exec.lastCmd, " ")
		assert.Contains(t, joined, "admin user create")
		assert.Contains(t, joined, "--username "+giteaAdminUser)
		assert.Contains(t, joined, "--config "+giteaConfigPath)
		// Must run as the unprivileged image account so files/DB stay correctly owned.
		require.NotNil(t, exec.lastOpt)
		assert.Equal(t, giteaRunAsUser, exec.lastOpt.User)
	})

	t.Run("idempotent when the user already exists", func(t *testing.T) {
		// Gitea reports "user already exists" on a repeat run; treat it as success.
		exec := &fakeGiteaExecer{err: errors.New("exit status 1"), stderr: "Command error: user already exists [name: atmos]"}
		assert.NoError(t, ensureGiteaAdmin(context.Background(), exec, "cid"))
	})

	t.Run("propagates a genuine failure", func(t *testing.T) {
		exec := &fakeGiteaExecer{err: errors.New("exit status 1"), stderr: "permission denied"}
		assert.Error(t, ensureGiteaAdmin(context.Background(), exec, "cid"))
	})
}

func TestEnsureGiteaRepo(t *testing.T) {
	t.Run("created (201) is success and uses admin basic auth", func(t *testing.T) {
		var gotAuth bool
		var gotPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			gotAuth = ok && user == giteaAdminUser && pass == giteaAdminPassword
			gotPath = r.URL.Path
			w.WriteHeader(http.StatusCreated)
		}))
		defer srv.Close()

		require.NoError(t, ensureGiteaRepo(context.Background(), srv.URL))
		assert.True(t, gotAuth, "must authenticate as the gitea admin")
		assert.Equal(t, "/api/v1/user/repos", gotPath)
	})

	t.Run("conflict (409) is an idempotent success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusConflict)
		}))
		defer srv.Close()

		assert.NoError(t, ensureGiteaRepo(context.Background(), srv.URL))
	})

	t.Run("cancelled context aborts the retry loop", func(t *testing.T) {
		// A 500 keeps the loop retrying; a cancelled context must end it promptly.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		assert.Error(t, ensureGiteaRepo(ctx, srv.URL))
	})
}
