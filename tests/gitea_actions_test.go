package tests

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
)

// The gitea-actions example is both the human-runnable demo and this test's fixture:
// its harness (atmos.yaml, stacks/) starts Gitea plus a runner, and its repo/
// directory is the project pushed to Gitea, including the recommended workflows.
const (
	giteaActionsExampleDir = "../examples/gitea-actions"
	giteaActionsStack      = "local"
	giteaActionsURL        = "http://localhost:3300"
	// The job image tag. Jobs run this branch's atmos, bind-mounted over the image's.
	giteaActionsAtmosImageVersion = "1.234.0"
	giteaActionsRunTimeout        = 10 * time.Minute
)

// requireGiteaActionsRuntime gates the test on the floci opt-in flag, a container
// runtime, and a Docker socket at the path the example's runner mounts.
func requireGiteaActionsRuntime(t *testing.T) {
	t.Helper()
	if os.Getenv("ATMOS_TEST_FLOCI") != "true" {
		t.Skip("set ATMOS_TEST_FLOCI=true to run the Gitea Actions E2E")
	}
	if _, err := container.DetectRuntime(context.Background()); err != nil {
		t.Skipf("no container runtime available for the Gitea Actions E2E: %v", err)
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		t.Skipf("the Gitea runner needs a Docker socket at /var/run/docker.sock: %v", err)
	}
}

// TestGiteaActionsE2E runs the recommended native CI workflows, adapted for Gitea,
// on a real Gitea server and runner: apply on push, plan and deploy-affected on a
// pull request, and deploy-all on workflow_dispatch. It asserts the workflows pass
// and that Atmos reported through the Gitea provider (commit statuses and a
// marker-based pull request comment).
//
// Opt-in and runtime-gated (ATMOS_TEST_FLOCI=true, a container runtime, and a
// Docker socket). It pulls the Gitea, runner, and Atmos images.
func TestGiteaActionsE2E(t *testing.T) {
	requireGiteaActionsRuntime(t)
	ensureAtmosRunner(t)
	requireGitBinary(t)

	workdir := copyFlociScenarioFixture(t, giteaActionsExampleDir)
	mountAtmosIntoJobs(t, filepath.Join(workdir, "components", "container", "gitea-runner", "runner.yaml"), buildLinuxAtmos(t))
	env := map[string]string{"ATMOS_CLI_CONFIG_PATH": workdir}
	run := func(timeout time.Duration, args ...string) {
		t.Helper()
		_, stderr, err := runFlociAtmosInDir(t, workdir, env, timeout, args...)
		require.NoErrorf(t, err, "atmos %s failed: %s", strings.Join(args, " "), stderr)
	}

	// Start Gitea, then the runner that registers with it. Always tear both down.
	run(5*time.Minute, "emulator", "up", "gitea", "-s", giteaActionsStack)
	t.Cleanup(func() {
		_, _, _ = runFlociAtmosInDir(t, workdir, env, 2*time.Minute, "emulator", "down", "gitea", "-s", giteaActionsStack)
	})
	run(5*time.Minute, "container", "up", "gitea-runner", "-s", giteaActionsStack)
	t.Cleanup(func() {
		_, _, _ = runFlociAtmosInDir(t, workdir, env, 2*time.Minute, "container", "down", "gitea-runner", "-s", giteaActionsStack)
	})

	api := giteaAPI{base: giteaActionsURL}
	seedGiteaActionsRepo(t, api)

	// Push the project to main: apply.yml deploys prod.
	repo := filepath.Join(t.TempDir(), "infra")
	require.NoError(t, copyFlociFixture(filepath.Join(workdir, "repo"), repo))
	commit := func(message string) {
		runGit(t, repo, "-c", "user.name=Atmos", "-c", "user.email=atmos@localhost", "commit", "-qam", message)
	}
	remote := "http://atmos:atmos@localhost:3300/atmos/infra.git"
	runGit(t, repo, "init", "-q", "-b", "main")
	runGit(t, repo, "add", "-A")
	commit("Add infrastructure")
	runGit(t, repo, "push", "-q", remote, "main")
	mainSHA := gitOutput(t, repo, "rev-parse", "HEAD")
	api.waitForRun(t, "apply.yml", "push", mainSHA)
	require.Equal(t, "success", api.statuses(t, mainSHA)["atmos/deploy/prod/demo"].Status)

	// Change only dev in a pull request: plan.yml plans prod and posts a comment;
	// deploy-affected.yml deploys only the affected dev instance.
	runGit(t, repo, "checkout", "-q", "-b", "change-dev")
	devStack := filepath.Join(repo, "stacks", "deploy", "dev.yaml")
	content, err := os.ReadFile(devStack)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(devStack, []byte(strings.Replace(string(content), "Hello from dev", "Hello again from dev", 1)), 0o600))
	commit("Change the dev message")
	runGit(t, repo, "push", "-q", remote, "change-dev")
	headSHA := gitOutput(t, repo, "rev-parse", "HEAD")
	var pr struct {
		Number int `json:"number"`
	}
	api.mustDo(t, http.StatusCreated, http.MethodPost, "/repos/atmos/infra/pulls",
		map[string]string{"title": "Change the dev message", "head": "change-dev", "base": "main"}, &pr)
	api.waitForRun(t, "plan.yml", "pull_request", headSHA)
	api.waitForRun(t, "deploy-affected.yml", "pull_request", headSHA)

	statuses := api.statuses(t, headSHA)
	require.Equal(t, "success", statuses["atmos/plan/prod/demo"].Status, "plan should report a commit status")
	require.Equal(t, "success", statuses["atmos/deploy/dev/demo"].Status, "the affected dev instance should deploy")
	require.NotContains(t, statuses, "atmos/deploy/prod/demo", "the unaffected prod instance must not deploy")
	requirePlanComment(t, api, pr.Number)

	// Dispatch deploy-all on main: it deploys every instance.
	api.mustDo(t, http.StatusNoContent, http.MethodPost, "/repos/atmos/infra/actions/workflows/deploy-all.yml/dispatches",
		map[string]string{"ref": "main"}, nil)
	api.waitForRun(t, "deploy-all.yml", "workflow_dispatch", mainSHA)
	statuses = api.statuses(t, mainSHA)
	require.Equal(t, "success", statuses["atmos/deploy/dev/demo"].Status)
	require.Equal(t, "success", statuses["atmos/deploy/prod/demo"].Status)
}

// seedGiteaActionsRepo creates the infra repository with the Actions secret and
// variable the workflows read: a scoped token for Atmos and the job image version.
func seedGiteaActionsRepo(t *testing.T, api giteaAPI) {
	t.Helper()
	api.mustDo(t, http.StatusCreated, http.MethodPost, "/user/repos",
		map[string]any{"name": "infra", "default_branch": "main", "private": false}, nil)
	var token struct {
		SHA1 string `json:"sha1"`
	}
	api.mustDo(t, http.StatusCreated, http.MethodPost, "/users/atmos/tokens",
		map[string]any{"name": "atmos-ci", "scopes": []string{"write:repository", "write:issue", "read:user"}}, &token)
	require.NotEmpty(t, token.SHA1)
	api.mustDo(t, http.StatusCreated, http.MethodPut, "/repos/atmos/infra/actions/secrets/ATMOS_CI_GITEA_TOKEN",
		map[string]string{"data": token.SHA1}, nil)
	api.mustDo(t, http.StatusCreated, http.MethodPost, "/repos/atmos/infra/actions/variables/ATMOS_VERSION",
		map[string]string{"value": giteaActionsAtmosImageVersion}, nil)
}

// requirePlanComment asserts Atmos posted exactly one marker-tagged plan comment.
func requirePlanComment(t *testing.T, api giteaAPI, number int) {
	t.Helper()
	var comments []struct {
		Body string `json:"body"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	api.mustDo(t, http.StatusOK, http.MethodGet, "/repos/atmos/infra/issues/"+strconv.Itoa(number)+"/comments", nil, &comments)
	found := 0
	for _, c := range comments {
		if strings.Contains(c.Body, "<!-- atmos:ci:plan:demo:prod -->") {
			found++
			require.Equal(t, "atmos", c.User.Login)
			require.Contains(t, c.Body, "demo")
		}
	}
	require.Equal(t, 1, found, "expected one Atmos plan comment on the pull request")
}
