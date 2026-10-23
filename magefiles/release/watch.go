package release

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
)

// releaseWatcher keeps terminal progress separate from authoritative API checks.
type releaseWatcher interface {
	Check(context.Context) error
	Watch(context.Context, string, int64, io.Writer) error
}

type ghReleaseWatcher struct{}

// Check rejects unavailable or unsupported watcher credentials before dispatch.
func (ghReleaseWatcher) Check(ctx context.Context) error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("GitHub CLI (gh) is required to watch releases; install it before dispatching: %w", err)
	}
	// Resolve the same environment/config/keyring credential as gh run watch.
	// Capture the token privately; never attach this subprocess to the terminal.
	token, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err != nil {
		return fmt.Errorf("%w: cannot read GitHub CLI credentials; run gh auth login before dispatching: %w", ErrReleaseVerification, errors.Join(err, ctx.Err()))
	}
	token = bytes.TrimSpace(token)
	if len(token) == 0 {
		return fmt.Errorf("%w: no GitHub CLI credential; run gh auth login before dispatching", ErrReleaseVerification)
	}
	if bytes.HasPrefix(token, []byte("github_pat_")) {
		return fmt.Errorf("%w: gh run watch does not support fine-grained PATs; select a supported GitHub CLI credential (gh auth login or a classic PAT) before dispatching; check GH_TOKEN and GITHUB_TOKEN overrides", ErrReleaseVerification)
	}
	return nil
}

func (ghReleaseWatcher) Watch(ctx context.Context, repo string, runID int64, out io.Writer) error {
	if runID <= 0 || !githubapi.RepoPattern.MatchString(repo) {
		return fmt.Errorf("%w: invalid workflow watch identity", ErrReleaseVerification)
	}
	// #nosec G204 -- Fixed executable/subcommand, validated repository and numeric run ID; no shell.
	cmd := exec.CommandContext(ctx, "gh", "run", "watch", strconv.FormatInt(runID, decimalBase), "--repo", repo, "--exit-status")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, out, os.Stderr
	if err := cmd.Run(); err != nil {
		return errors.Join(err, ctx.Err())
	}
	return nil
}

func (c *milestoneClient) watchRun(runID int64, out io.Writer) error {
	if err := c.watcher.Watch(c.Context, c.Repo, runID, out); err != nil {
		return fmt.Errorf("watch stopped for %s; the remote workflow was not cancelled or redispatched; reconnect with `gh run watch %d --repo %s --exit-status`: %w", c.releaseRunURL(runID), runID, c.Repo, err)
	}
	complete, err := c.releaseRunComplete(runID)
	if err != nil {
		return err
	}
	if !complete {
		return fmt.Errorf("%w: watch exited before %s completed; reconnect with `gh run watch %d --repo %s --exit-status`", ErrReleaseVerification, c.releaseRunURL(runID), runID, c.Repo)
	}
	_, err = fmt.Fprintf(out, "Release workflow succeeded: %s\n", c.releaseRunURL(runID))
	return err
}
