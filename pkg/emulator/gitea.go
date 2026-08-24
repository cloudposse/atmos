package emulator

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Gitea bootstrap constants. A fresh Gitea boots installed-but-empty (the driver
// sets INSTALL_LOCK so the web wizard is skipped), so the manager creates the
// admin user and the deployment repository — mirroring the Vault/OpenBao
// bootstrap. The credentials are throwaway-local: they exist only to make a
// self-contained GitOps loop work and are embedded in the configured remote URL,
// so there is no real secret to protect.
const (
	// The giteaAdminUser / giteaAdminPassword are the throwaway-local admin
	// credentials the bootstrap creates and the example's remote URL embeds.
	giteaAdminUser     = "atmos"
	giteaAdminPassword = "atmos"
	// The giteaAdminEmail is a placeholder address (email is disabled in the demo).
	giteaAdminEmail = "atmos@localhost"
	// The giteaDeploymentsRepo is the repository the GitOps loop pushes manifests to.
	giteaDeploymentsRepo = "deployments"
	// The giteaConfigPath is the in-container app.ini location for the standard
	// gitea/gitea image (GITEA_CUSTOM=/data/gitea). The admin-create CLI needs it.
	giteaConfigPath = "/data/gitea/conf/app.ini"
	// The giteaWorkDir is the in-container working directory for the gitea binary.
	giteaWorkDir = "/data/gitea"
	// The giteaRunAsUser is the unprivileged account Gitea runs as in the image; the
	// admin-create CLI must run as this user so DB/files stay correctly owned.
	giteaRunAsUser = "git"
	// The giteaReadyTimeout / giteaReadyInterval bound the repo-creation API retry loop.
	giteaReadyTimeout  = 60 * time.Second
	giteaReadyInterval = time.Second
	// The giteaHTTPTimeout bounds each local repo-create API request.
	giteaHTTPTimeout = 10 * time.Second
)

// giteaExecer is the subset of container.Runtime the git bootstrap needs (Exec).
// It keeps the helpers testable and the dependency explicit (mirrors vaultExecer).
type giteaExecer interface {
	Exec(ctx context.Context, containerID string, cmd []string, opts *container.ExecOptions) error
}

// bootstrapGitea makes a running Gitea server ready for the GitOps loop: it
// creates the local admin user (via the in-container CLI) and the deployment
// repository (via the API, auto-initialized so it is immediately clonable). It is
// idempotent — an already-present user or repo is a clean success — so `up` after
// `up` is a no-op.
func bootstrapGitea(ctx context.Context, runtime giteaExecer, containerID, baseURL string) error {
	defer perf.Track(nil, "emulator.bootstrapGitea")()

	if err := ensureGiteaAdmin(ctx, runtime, containerID); err != nil {
		return err
	}
	return ensureGiteaRepo(ctx, baseURL)
}

// ensureGiteaAdmin creates the local admin user with the gitea CLI, running as the
// image's `git` account against the installed app.ini. Gitea reports "user already
// exists" on a repeat run, which the bootstrap treats as success for idempotency.
func ensureGiteaAdmin(ctx context.Context, runtime giteaExecer, containerID string) error {
	cmd := []string{
		"gitea", "admin", "user", "create",
		"--admin",
		"--username", giteaAdminUser,
		"--password", giteaAdminPassword,
		"--email", giteaAdminEmail,
		"--must-change-password=false",
		"--config", giteaConfigPath,
	}
	var out bytes.Buffer
	err := runtime.Exec(ctx, containerID, cmd, &container.ExecOptions{
		User:         giteaRunAsUser,
		Env:          []string{"GITEA_WORK_DIR=" + giteaWorkDir},
		AttachStdout: true,
		AttachStderr: true,
		Stdout:       &out,
		Stderr:       &out,
	})
	if err == nil {
		return nil
	}
	// A repeat bootstrap is a clean success: the user already exists.
	if strings.Contains(strings.ToLower(out.String()), "already exists") {
		return nil
	}
	return fmt.Errorf("%w: create gitea admin user: %w: %s", errUtils.ErrEmulatorConfigInvalid, err, strings.TrimSpace(out.String()))
}

// The ensureGiteaRepo creates the auto-initialized deployment repository via the
// Gitea API using the admin's basic auth, retrying until the API accepts the
// request. A 201 (created) and a 409 (already exists) are both success, so the
// call is idempotent. The auto_init flag gives the repo an initial commit on the
// default branch so the git provisioner's clone-reconcile has something to clone.
func ensureGiteaRepo(ctx context.Context, baseURL string) error {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/api/v1/user/repos"
	body := fmt.Sprintf(`{"name":%q,"auto_init":true,"default_branch":"main","private":false}`, giteaDeploymentsRepo)

	deadline := time.Now().Add(giteaReadyTimeout)
	var lastErr error
	for {
		status, rerr := postGiteaRepo(ctx, endpoint, body)
		done, lerr := classifyRepoCreate(status, rerr)
		if done {
			return nil
		}
		lastErr = lerr
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: gitea repo %q not created within %s: %w", errUtils.ErrEmulatorConfigInvalid, giteaDeploymentsRepo, giteaReadyTimeout, lastErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(giteaReadyInterval):
		}
	}
}

// The classifyRepoCreate maps one repo-create attempt to (done, retryableErr): a
// 201/409 is success (done); anything else — including the admin-not-yet-propagated
// 401/403 — is a retryable error carried until the deadline.
func classifyRepoCreate(status int, rerr error) (bool, error) {
	if rerr != nil {
		return false, rerr
	}
	if status == http.StatusCreated || status == http.StatusConflict {
		return true, nil
	}
	return false, fmt.Errorf("%w: gitea repo create returned %d", errUtils.ErrEmulatorConfigInvalid, status)
}

// The postGiteaRepo issues the authenticated repo-create request and returns the
// HTTP status code (or an error when the request itself could not be completed).
func postGiteaRepo(ctx context.Context, endpoint, body string) (int, error) {
	// The endpoint is the local emulator's own host:port (built from the resolved
	// container endpoint), not user-supplied input, so the variable URL is safe.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body)) //nolint:noctx // ctx is passed via NewRequestWithContext.
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(giteaAdminUser, giteaAdminPassword)

	client := &http.Client{Timeout: giteaHTTPTimeout}
	resp, err := client.Do(req) //nolint:gosec // G704: endpoint is the local emulator host:port, not user input.
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}
