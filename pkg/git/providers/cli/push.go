package cli

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Push pushes the current branch, retrying on non-fast-forward rejection —
// the most common GitOps publishing failure (a concurrent publisher pushed
// first). Each retry rebases local commits onto the updated remote ref and
// pushes again, bounded by opts.Retries. Atmos never force-pushes.
func (p *Provider) Push(ctx context.Context, opts *atmosgit.PushOptions) error {
	defer perf.Track(nil, "cli.Provider.Push")()

	attempts := opts.Retries + 1
	for attempt := range attempts {
		result, err := p.pushOnce(ctx, opts.RepoContext)
		if err == nil {
			return nil
		}
		if !isRejectedPush(result) {
			return classify(err, result, "push")
		}
		if attempt == attempts-1 {
			break
		}
		if err := p.rebaseOntoRemote(ctx, opts.RepoContext); err != nil {
			return err
		}
	}

	return fmt.Errorf("%w: %s/%s after %d retr%s", errUtils.ErrGitPushRejected,
		remoteOrDefault(opts.Remote), opts.Branch, opts.Retries, pluralRetry(opts.Retries))
}

// pushOnce performs a single push attempt.
func (p *Provider) pushOnce(ctx context.Context, rc atmosgit.RepoContext) (atmosgit.RunResult, error) {
	args := []string{"push", remoteOrDefault(rc.Remote)}
	if rc.Branch != "" {
		args = append(args, rc.Branch)
	}
	return p.run(ctx, rc.Workdir, rc.Env, args...)
}

// rebaseOntoRemote replays local commits onto the updated remote ref so the
// next push attempt is fast-forward for the remote.
func (p *Provider) rebaseOntoRemote(ctx context.Context, rc atmosgit.RepoContext) error {
	args := []string{"pull", "--rebase", remoteOrDefault(rc.Remote)}
	if rc.Branch != "" {
		args = append(args, rc.Branch)
	}

	result, err := p.run(ctx, rc.Workdir, rc.Env, args...)
	if err != nil {
		return classify(err, result, "pull --rebase")
	}
	return nil
}

// pluralRetry pluralizes "retry" for error messages.
func pluralRetry(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
