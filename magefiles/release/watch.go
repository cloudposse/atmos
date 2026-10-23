package release

import (
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
	Check() error
	Watch(context.Context, string, int64, io.Writer) error
}

type ghReleaseWatcher struct{}

func (ghReleaseWatcher) Check() error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("GitHub CLI (gh) is required to watch releases; install it before dispatching: %w", err)
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
